// Package almacen persiste eventos y estado actual en SQLite.
package almacen

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/envios"

	_ "modernc.org/sqlite" // driver sin cgo, para que corra en cualquier maquina
)

//go:embed esquema.sql
var esquema string

// ErrNoEncontrado se devuelve cuando el envio no existe.
var ErrNoEncontrado = errors.New("envio no encontrado")

// Resultado dice que paso con un evento recibido.
type Resultado string

const (
	Aplicado      Resultado = "aplicado"       // nuevo y cambio el estado actual
	SoloHistorial Resultado = "solo_historial" // nuevo, pero mas viejo que el estado actual
	Duplicado     Resultado = "duplicado"      // ese event_id ya estaba
)

// Almacen guarda eventos y estado actual.
type Almacen struct {
	db  *sql.DB
	now func() time.Time
}

// Abrir abre (o crea) la base y aplica el esquema.
func Abrir(ctx context.Context, dsn string) (*Almacen, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	// SQLite serializa las escrituras; con una sola conexion la transaccion de
	// RegistrarEvento (leer estado, calcular, escribir) no se pisa con otra.
	// En PostgreSQL esto seria un SELECT ... FOR UPDATE sobre el envio.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, esquema); err != nil {
		db.Close()
		return nil, fmt.Errorf("aplicar esquema: %w", err)
	}
	return &Almacen{db: db, now: time.Now}, nil
}

// Cerrar libera la base.
func (a *Almacen) Cerrar() error { return a.db.Close() }

// RegistrarEvento guarda el evento en el historial y, si corresponde, actualiza
// el estado actual. Todo en una transaccion: o quedan las dos cosas o ninguna.
func (a *Almacen) RegistrarEvento(ctx context.Context, ev envios.Evento) (Resultado, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("iniciar transaccion: %w", err)
	}
	defer tx.Rollback()

	ahora := a.now().UTC()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO envio_evento (event_id, shipment_id, estado, ocurrido_en, fecha_entrega_estimada, recibido_en)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (event_id) DO NOTHING`,
		ev.EventID, ev.ShipmentID, string(ev.Estado), fmtTiempo(ev.OcurridoEn),
		fmtTiempoPtr(ev.FechaEntregaEstimada), fmtTiempo(ahora))
	if err != nil {
		return "", fmt.Errorf("insertar evento: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("filas insertadas: %w", err)
	}
	if n == 0 {
		return Duplicado, nil
	}

	actual, err := buscarEnvio(ctx, tx, ev.ShipmentID)
	if err != nil && !errors.Is(err, ErrNoEncontrado) {
		return "", err
	}
	sig := envios.Aplicar(actual, ev, ahora)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO envio (shipment_id, estado, estado_desde, fecha_entrega_estimada, eta_informada_en, entregado_en, actualizado_en)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (shipment_id) DO UPDATE SET
			estado = excluded.estado,
			estado_desde = excluded.estado_desde,
			fecha_entrega_estimada = excluded.fecha_entrega_estimada,
			eta_informada_en = excluded.eta_informada_en,
			entregado_en = excluded.entregado_en,
			actualizado_en = excluded.actualizado_en`,
		sig.ShipmentID, string(sig.Estado), fmtTiempo(sig.EstadoDesde),
		fmtTiempoPtr(sig.FechaEntregaEstimada), fmtTiempoPtr(sig.ETAInformadaEn),
		fmtTiempoPtr(sig.EntregadoEn), fmtTiempo(sig.ActualizadoEn)); err != nil {
		return "", fmt.Errorf("guardar envio: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	if actual != nil && sig.EstadoDesde.Equal(actual.EstadoDesde) {
		return SoloHistorial, nil
	}
	return Aplicado, nil
}

// BuscarEnvio devuelve el estado actual de un envio.
func (a *Almacen) BuscarEnvio(ctx context.Context, shipmentID string) (envios.Envio, error) {
	e, err := buscarEnvio(ctx, a.db, shipmentID)
	if err != nil {
		return envios.Envio{}, err
	}
	return *e, nil
}

// Historial devuelve todos los eventos del envio en orden cronologico segun
// cuando ocurrieron (no segun cuando llegaron), como pidio el PO.
func (a *Almacen) Historial(ctx context.Context, shipmentID string) ([]envios.Evento, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT event_id, shipment_id, estado, ocurrido_en, fecha_entrega_estimada, recibido_en
		FROM envio_evento
		WHERE shipment_id = ?`, shipmentID)
	if err != nil {
		return nil, fmt.Errorf("consultar historial: %w", err)
	}
	defer rows.Close()

	eventos := []envios.Evento{}
	for rows.Next() {
		var ev envios.Evento
		var estado, ocurrido, recibido string
		var eta sql.NullString
		if err := rows.Scan(&ev.EventID, &ev.ShipmentID, &estado, &ocurrido, &eta, &recibido); err != nil {
			return nil, fmt.Errorf("leer evento: %w", err)
		}
		ev.Estado = envios.Estado(estado)
		if ev.OcurridoEn, err = parseTiempo(ocurrido); err != nil {
			return nil, err
		}
		if ev.RecibidoEn, err = parseTiempo(recibido); err != nil {
			return nil, err
		}
		if ev.FechaEntregaEstimada, err = parseTiempoNull(eta); err != nil {
			return nil, err
		}
		eventos = append(eventos, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recorrer historial: %w", err)
	}
	// Se ordena en Go y no con ORDER BY sobre el texto: RFC 3339 con
	// fracciones de segundo variables no ordena bien como string.
	ordenarCronologico(eventos)
	return eventos, nil
}

type consultor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func buscarEnvio(ctx context.Context, q consultor, shipmentID string) (*envios.Envio, error) {
	var e envios.Envio
	var estado, desde, actualizado string
	var eta, etaEn, entregado sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT shipment_id, estado, estado_desde, fecha_entrega_estimada, eta_informada_en, entregado_en, actualizado_en
		FROM envio WHERE shipment_id = ?`, shipmentID).
		Scan(&e.ShipmentID, &estado, &desde, &eta, &etaEn, &entregado, &actualizado)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("buscar envio: %w", err)
	}
	e.Estado = envios.Estado(estado)
	if e.EstadoDesde, err = parseTiempo(desde); err != nil {
		return nil, err
	}
	if e.ActualizadoEn, err = parseTiempo(actualizado); err != nil {
		return nil, err
	}
	if e.FechaEntregaEstimada, err = parseTiempoNull(eta); err != nil {
		return nil, err
	}
	if e.ETAInformadaEn, err = parseTiempoNull(etaEn); err != nil {
		return nil, err
	}
	if e.EntregadoEn, err = parseTiempoNull(entregado); err != nil {
		return nil, err
	}
	return &e, nil
}
