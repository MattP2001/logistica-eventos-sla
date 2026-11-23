package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/almacen"
)

// nuevoServidor levanta la API real contra un SQLite en memoria.
func nuevoServidor(t *testing.T, ahora time.Time) *httptest.Server {
	t.Helper()
	// Con una sola conexion (ver almacen.Abrir), ":memory:" es una base nueva
	// por test y vive mientras el test la use.
	a, err := almacen.Abrir(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Cerrar() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(Nuevo(a, func() time.Time { return ahora }, log).Rutas())
	t.Cleanup(srv.Close)
	return srv
}

func postEvento(t *testing.T, srv *httptest.Server, cuerpo string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/webhooks/operador/eventos", "application/json", strings.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getJSON(t *testing.T, srv *httptest.Server, ruta string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(srv.URL + ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestWebhook_DuplicadoNoSeGuardaDosVeces(t *testing.T) {
	srv := nuevoServidor(t, time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC))
	ev := `{"event_id":"e1","shipment_id":"S1","status":"CREADO","timestamp":"2026-11-23T08:00:00-03:00","estimated_delivery_date":"2026-11-25T18:00:00-03:00"}`

	if code, out := postEvento(t, srv, ev); code != 200 || out["resultado"] != "aplicado" {
		t.Fatalf("primer envio: %d %v", code, out)
	}
	if code, out := postEvento(t, srv, ev); code != 200 || out["resultado"] != "duplicado" {
		t.Fatalf("reenvio: %d %v", code, out)
	}

	_, hist := getJSON(t, srv, "/envios/S1/eventos")
	if n := len(hist["eventos"].([]any)); n != 1 {
		t.Fatalf("el historial tiene %d eventos, queria 1", n)
	}
}

func TestWebhook_FueraDeOrdenQuedaEnHistorialPeroNoPisaElEstado(t *testing.T) {
	srv := nuevoServidor(t, time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC))

	postEvento(t, srv, `{"event_id":"e3","shipment_id":"S1","status":"EN_DISTRIBUCION","timestamp":"2026-11-23T12:00:00-03:00"}`)
	_, out := postEvento(t, srv, `{"event_id":"e2","shipment_id":"S1","status":"EN_TRANSITO","timestamp":"2026-11-23T09:00:00-03:00"}`)
	if out["resultado"] != "solo_historial" {
		t.Fatalf("resultado = %v", out["resultado"])
	}

	_, envio := getJSON(t, srv, "/envios/S1")
	if envio["estado"] != "EN_DISTRIBUCION" {
		t.Fatalf("estado = %v, queria EN_DISTRIBUCION", envio["estado"])
	}

	_, hist := getJSON(t, srv, "/envios/S1/eventos")
	evs := hist["eventos"].([]any)
	if len(evs) != 2 {
		t.Fatalf("historial con %d eventos", len(evs))
	}
	if primero := evs[0].(map[string]any)["event_id"]; primero != "e2" {
		t.Fatalf("el historial no esta en orden cronologico: primero %v", primero)
	}
}

func TestConsulta_DemoradoSeCalculaContraLaHoraActual(t *testing.T) {
	// "Ahora" es posterior a la fecha estimada y el envio sigue en transito.
	srv := nuevoServidor(t, time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC))
	postEvento(t, srv, `{"event_id":"e1","shipment_id":"S1","status":"EN_TRANSITO","timestamp":"2026-11-23T08:00:00-03:00","estimated_delivery_date":"2026-11-25T18:00:00-03:00"}`)

	_, envio := getJSON(t, srv, "/envios/S1")
	if envio["demorado"] != true {
		t.Fatalf("demorado = %v, queria true", envio["demorado"])
	}
}

func TestConsulta_EntregadoConDemora(t *testing.T) {
	srv := nuevoServidor(t, time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC))
	postEvento(t, srv, `{"event_id":"e1","shipment_id":"S1","status":"CREADO","timestamp":"2026-11-23T08:00:00-03:00","estimated_delivery_date":"2026-11-25T18:00:00-03:00"}`)
	postEvento(t, srv, `{"event_id":"e2","shipment_id":"S1","status":"ENTREGADO","timestamp":"2026-11-26T10:00:00-03:00"}`)

	_, envio := getJSON(t, srv, "/envios/S1")
	if envio["estado"] != "ENTREGADO" || envio["demorado"] != false || envio["entregado_con_demora"] != true {
		t.Fatalf("got %v", envio)
	}
}

func TestWebhook_Validaciones(t *testing.T) {
	srv := nuevoServidor(t, time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC))
	casos := map[string]string{
		"json roto":          `{"event_id":`,
		"sin event_id":       `{"shipment_id":"S1","status":"CREADO","timestamp":"2026-11-23T08:00:00Z"}`,
		"status desconocido": `{"event_id":"e1","shipment_id":"S1","status":"PERDIDO","timestamp":"2026-11-23T08:00:00Z"}`,
		"fecha sin zona":     `{"event_id":"e1","shipment_id":"S1","status":"CREADO","timestamp":"2026-11-23T08:00:00"}`,
	}
	for nombre, cuerpo := range casos {
		t.Run(nombre, func(t *testing.T) {
			if code, _ := postEvento(t, srv, cuerpo); code != http.StatusBadRequest {
				t.Fatalf("status = %d, queria 400", code)
			}
		})
	}
}

func TestConsulta_EnvioInexistente(t *testing.T) {
	srv := nuevoServidor(t, time.Now())
	if code, _ := getJSON(t, srv, "/envios/NOEXISTE"); code != http.StatusNotFound {
		t.Fatalf("status = %d", code)
	}
	if code, _ := getJSON(t, srv, "/envios/NOEXISTE/eventos"); code != http.StatusNotFound {
		t.Fatalf("status = %d", code)
	}
}
