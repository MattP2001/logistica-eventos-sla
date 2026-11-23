package almacen

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/envios"
)

func abrirEnMemoria(t *testing.T) *Almacen {
	t.Helper()
	a, err := Abrir(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Cerrar() })
	return a
}

func TestRegistrarEvento_ResultadosYEstadoPersistido(t *testing.T) {
	ctx := context.Background()
	a := abrirEnMemoria(t)
	eta := time.Date(2026, 11, 25, 21, 0, 0, 0, time.UTC)
	base := time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC)

	pasos := []struct {
		ev     envios.Evento
		quiero Resultado
	}{
		{envios.Evento{EventID: "e1", ShipmentID: "S1", Estado: envios.Creado, OcurridoEn: base, FechaEntregaEstimada: &eta}, Aplicado},
		{envios.Evento{EventID: "e1", ShipmentID: "S1", Estado: envios.Creado, OcurridoEn: base, FechaEntregaEstimada: &eta}, Duplicado},
		{envios.Evento{EventID: "e3", ShipmentID: "S1", Estado: envios.EnDistribucion, OcurridoEn: base.Add(5 * time.Hour)}, Aplicado},
		{envios.Evento{EventID: "e2", ShipmentID: "S1", Estado: envios.EnTransito, OcurridoEn: base.Add(2 * time.Hour)}, SoloHistorial},
	}
	for _, p := range pasos {
		got, err := a.RegistrarEvento(ctx, p.ev)
		if err != nil {
			t.Fatal(err)
		}
		if got != p.quiero {
			t.Fatalf("%s: resultado %s, queria %s", p.ev.EventID, got, p.quiero)
		}
	}

	e, err := a.BuscarEnvio(ctx, "S1")
	if err != nil {
		t.Fatal(err)
	}
	if e.Estado != envios.EnDistribucion || e.FechaEntregaEstimada == nil || !e.FechaEntregaEstimada.Equal(eta) {
		t.Fatalf("envio guardado: %+v", e)
	}

	hist, err := a.Historial(ctx, "S1")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ev := range hist {
		ids = append(ids, ev.EventID)
	}
	if len(ids) != 3 || ids[0] != "e1" || ids[1] != "e2" || ids[2] != "e3" {
		t.Fatalf("historial = %v, queria [e1 e2 e3]", ids)
	}
}

func TestBuscarEnvio_NoEncontrado(t *testing.T) {
	a := abrirEnMemoria(t)
	if _, err := a.BuscarEnvio(context.Background(), "nada"); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("err = %v", err)
	}
}
