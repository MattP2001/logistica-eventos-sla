package envios

import (
	"testing"
	"time"
)

func ts(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ptr(v time.Time) *time.Time { return &v }

func TestAplicar_EnvioNuevoAunqueNoSeaCreado(t *testing.T) {
	ahora := ts(t, "2026-11-23T20:00:00Z")
	ev := Evento{EventID: "e2", ShipmentID: "S1", Estado: EnTransito, OcurridoEn: ts(t, "2026-11-23T10:00:00Z")}

	got := Aplicar(nil, ev, ahora)

	if got.Estado != EnTransito || !got.EstadoDesde.Equal(ev.OcurridoEn) {
		t.Fatalf("estado = %s desde %v", got.Estado, got.EstadoDesde)
	}
}

func TestAplicar_EventoViejoNoPisaElEstado(t *testing.T) {
	ahora := ts(t, "2026-11-23T20:00:00Z")
	actual := Envio{ShipmentID: "S1", Estado: EnDistribucion, EstadoDesde: ts(t, "2026-11-23T12:00:00Z")}
	tarde := Evento{EventID: "e2", ShipmentID: "S1", Estado: EnTransito, OcurridoEn: ts(t, "2026-11-23T09:00:00Z")}

	got := Aplicar(&actual, tarde, ahora)

	if got.Estado != EnDistribucion {
		t.Fatalf("un evento viejo piso el estado: %s", got.Estado)
	}
}

func TestAplicar_GanaElTimestampMasReciente(t *testing.T) {
	ahora := ts(t, "2026-11-23T20:00:00Z")
	actual := Envio{ShipmentID: "S1", Estado: EnTransito, EstadoDesde: ts(t, "2026-11-23T09:00:00Z")}
	ev := Evento{EventID: "e3", ShipmentID: "S1", Estado: Entregado, OcurridoEn: ts(t, "2026-11-23T15:00:00Z")}

	got := Aplicar(&actual, ev, ahora)

	if got.Estado != Entregado || got.EntregadoEn == nil || !got.EntregadoEn.Equal(ev.OcurridoEn) {
		t.Fatalf("got %+v", got)
	}
}

func TestAplicar_TimestampIgualConservaElEstado(t *testing.T) {
	ahora := ts(t, "2026-11-23T20:00:00Z")
	mismo := ts(t, "2026-11-23T09:00:00Z")
	actual := Envio{ShipmentID: "S1", Estado: EnTransito, EstadoDesde: mismo}

	got := Aplicar(&actual, Evento{EventID: "x", ShipmentID: "S1", Estado: Cancelado, OcurridoEn: mismo}, ahora)

	if got.Estado != EnTransito {
		t.Fatalf("con timestamp igual cambio el estado a %s", got.Estado)
	}
}

func TestAplicar_FechaEstimadaViejaNoPisaLaRepactada(t *testing.T) {
	ahora := ts(t, "2026-11-23T20:00:00Z")
	repactada := ts(t, "2026-11-27T18:00:00Z")
	actual := Envio{
		ShipmentID: "S1", Estado: EnTransito, EstadoDesde: ts(t, "2026-11-23T12:00:00Z"),
		FechaEntregaEstimada: &repactada, ETAInformadaEn: ptr(ts(t, "2026-11-23T12:00:00Z")),
	}
	vieja := Evento{
		EventID: "e1", ShipmentID: "S1", Estado: Creado, OcurridoEn: ts(t, "2026-11-22T08:00:00Z"),
		FechaEntregaEstimada: ptr(ts(t, "2026-11-25T18:00:00Z")),
	}

	got := Aplicar(&actual, vieja, ahora)

	if !got.FechaEntregaEstimada.Equal(repactada) {
		t.Fatalf("fecha estimada = %v, queria %v", got.FechaEntregaEstimada, repactada)
	}
}
