// Package api expone el webhook del operador y las consultas de seguimiento.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/almacen"
	"github.com/MattP2001/logistica-eventos-sla/internal/envios"
)

// Almacen es lo que la API necesita de la persistencia.
type Almacen interface {
	RegistrarEvento(ctx context.Context, ev envios.Evento) (almacen.Resultado, error)
	BuscarEnvio(ctx context.Context, shipmentID string) (envios.Envio, error)
	Historial(ctx context.Context, shipmentID string) ([]envios.Evento, error)
}

// Server arma las rutas HTTP.
type Server struct {
	almacen Almacen
	ahora   func() time.Time
	log     *slog.Logger
}

// Nuevo crea el servidor. ahora se inyecta para poder testear la demora.
func Nuevo(a Almacen, ahora func() time.Time, log *slog.Logger) *Server {
	return &Server{almacen: a, ahora: ahora, log: log}
}

// Rutas devuelve el handler con todos los endpoints.
func (s *Server) Rutas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/operador/eventos", s.recibirEvento)
	mux.HandleFunc("GET /envios/{shipment_id}", s.consultarEnvio)
	mux.HandleFunc("GET /envios/{shipment_id}/eventos", s.consultarHistorial)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		escribirJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

// eventoEntrada es el payload del operador, tal como lo describio el PO.
type eventoEntrada struct {
	EventID               string  `json:"event_id"`
	ShipmentID            string  `json:"shipment_id"`
	Status                string  `json:"status"`
	Timestamp             string  `json:"timestamp"`
	EstimatedDeliveryDate *string `json:"estimated_delivery_date"`
}

const maxCuerpo = 64 << 10 // un evento son unos pocos cientos de bytes

func (s *Server) recibirEvento(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpo)
	dec := json.NewDecoder(r.Body)
	var in eventoEntrada
	if err := dec.Decode(&in); err != nil {
		escribirError(w, http.StatusBadRequest, "json_invalido", "el cuerpo no es un JSON valido")
		return
	}

	ev, err := in.aEvento()
	if err != nil {
		escribirError(w, http.StatusBadRequest, "evento_invalido", err.Error())
		return
	}

	res, err := s.almacen.RegistrarEvento(r.Context(), ev)
	if err != nil {
		// 500: es un problema nuestro y el operador puede reintentar; gracias
		// al event_id, el reintento no duplica nada.
		s.log.Error("registrar evento", "event_id", ev.EventID, "error", err)
		escribirError(w, http.StatusInternalServerError, "error_interno", "no pudimos registrar el evento")
		return
	}

	escribirJSON(w, http.StatusOK, map[string]string{"event_id": ev.EventID, "resultado": string(res)})
}

func (in eventoEntrada) aEvento() (envios.Evento, error) {
	ev := envios.Evento{
		EventID:    in.EventID,
		ShipmentID: in.ShipmentID,
		Estado:     envios.Estado(in.Status),
	}
	if in.Timestamp != "" {
		t, err := time.Parse(time.RFC3339, in.Timestamp)
		if err != nil {
			return ev, fmt.Errorf("%w: timestamp debe ser ISO 8601 con zona horaria", envios.ErrValidacion)
		}
		ev.OcurridoEn = t
	}
	if in.EstimatedDeliveryDate != nil && *in.EstimatedDeliveryDate != "" {
		t, err := time.Parse(time.RFC3339, *in.EstimatedDeliveryDate)
		if err != nil {
			return ev, fmt.Errorf("%w: estimated_delivery_date debe ser ISO 8601 con zona horaria", envios.ErrValidacion)
		}
		ev.FechaEntregaEstimada = &t
	}
	return ev, ev.Validar()
}

type envioSalida struct {
	ShipmentID           string     `json:"shipment_id"`
	Estado               string     `json:"estado"`
	EstadoDesde          time.Time  `json:"estado_desde"`
	FechaEntregaEstimada *time.Time `json:"fecha_entrega_estimada"`
	EntregadoEn          *time.Time `json:"entregado_en"`
	Demorado             bool       `json:"demorado"`
	EntregadoConDemora   bool       `json:"entregado_con_demora"`
}

func (s *Server) consultarEnvio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("shipment_id")
	e, err := s.almacen.BuscarEnvio(r.Context(), id)
	if errors.Is(err, almacen.ErrNoEncontrado) {
		escribirError(w, http.StatusNotFound, "envio_no_encontrado", "no hay eventos para ese envio")
		return
	}
	if err != nil {
		s.log.Error("consultar envio", "shipment_id", id, "error", err)
		escribirError(w, http.StatusInternalServerError, "error_interno", "no pudimos consultar el envio")
		return
	}
	// La demora se calcula al consultar, contra la hora actual: si el operador
	// deja de mandar eventos, el envio igual pasa a demorado cuando corresponde.
	escribirJSON(w, http.StatusOK, envioSalida{
		ShipmentID:           e.ShipmentID,
		Estado:               string(e.Estado),
		EstadoDesde:          e.EstadoDesde,
		FechaEntregaEstimada: e.FechaEntregaEstimada,
		EntregadoEn:          e.EntregadoEn,
		Demorado:             e.Demorado(s.ahora()),
		EntregadoConDemora:   e.EntregadoConDemora(),
	})
}

type eventoSalida struct {
	EventID              string     `json:"event_id"`
	Estado               string     `json:"estado"`
	OcurridoEn           time.Time  `json:"ocurrido_en"`
	FechaEntregaEstimada *time.Time `json:"fecha_entrega_estimada"`
	RecibidoEn           time.Time  `json:"recibido_en"`
}

func (s *Server) consultarHistorial(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("shipment_id")
	evs, err := s.almacen.Historial(r.Context(), id)
	if err != nil {
		s.log.Error("consultar historial", "shipment_id", id, "error", err)
		escribirError(w, http.StatusInternalServerError, "error_interno", "no pudimos consultar el historial")
		return
	}
	if len(evs) == 0 {
		escribirError(w, http.StatusNotFound, "envio_no_encontrado", "no hay eventos para ese envio")
		return
	}
	salida := make([]eventoSalida, 0, len(evs))
	for _, ev := range evs {
		salida = append(salida, eventoSalida{
			EventID:              ev.EventID,
			Estado:               string(ev.Estado),
			OcurridoEn:           ev.OcurridoEn,
			FechaEntregaEstimada: ev.FechaEntregaEstimada,
			RecibidoEn:           ev.RecibidoEn,
		})
	}
	escribirJSON(w, http.StatusOK, map[string]any{"shipment_id": id, "eventos": salida})
}

func escribirJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func escribirError(w http.ResponseWriter, status int, codigo, mensaje string) {
	escribirJSON(w, status, map[string]any{"error": map[string]string{"code": codigo, "message": mensaje}})
}
