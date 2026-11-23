// Package envios contiene el dominio: eventos del operador, estado actual de un
// envío y las reglas de demora acordadas con el PO (ver docs/NOTAS_PO.md).
package envios

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estado es el estado físico que informa el operador logístico.
type Estado string

const (
	Creado         Estado = "CREADO"
	EnTransito     Estado = "EN_TRANSITO"
	EnDistribucion Estado = "EN_DISTRIBUCION"
	Entregado      Estado = "ENTREGADO"
	Cancelado      Estado = "CANCELADO"
)

var estadosConocidos = map[Estado]bool{
	Creado: true, EnTransito: true, EnDistribucion: true, Entregado: true, Cancelado: true,
}

// EsConocido indica si el estado es uno de los cinco que acordamos con el PO.
func (e Estado) EsConocido() bool { return estadosConocidos[e] }

// Cerrado indica si el envío ya no puede demorarse (entregado o cancelado).
func (e Estado) Cerrado() bool { return e == Entregado || e == Cancelado }

// Evento es una novedad del operador tal como la persistimos.
type Evento struct {
	EventID               string
	ShipmentID            string
	Estado                Estado
	OcurridoEn            time.Time
	FechaEntregaEstimada  *time.Time
	RecibidoEn            time.Time
}

// ErrValidacion envuelve los errores de un payload inválido (se responde 400).
var ErrValidacion = errors.New("evento invalido")

// Validar revisa los campos obligatorios y que el estado sea conocido.
func (e Evento) Validar() error {
	var faltan []string
	if strings.TrimSpace(e.EventID) == "" {
		faltan = append(faltan, "event_id")
	}
	if strings.TrimSpace(e.ShipmentID) == "" {
		faltan = append(faltan, "shipment_id")
	}
	if e.OcurridoEn.IsZero() {
		faltan = append(faltan, "timestamp")
	}
	if len(faltan) > 0 {
		return fmt.Errorf("%w: faltan %s", ErrValidacion, strings.Join(faltan, ", "))
	}
	if !e.Estado.EsConocido() {
		return fmt.Errorf("%w: status %q desconocido", ErrValidacion, e.Estado)
	}
	return nil
}
