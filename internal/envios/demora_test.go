package envios

import "testing"

func TestDemorado(t *testing.T) {
	eta := ts(t, "2026-11-23T18:00:00Z")
	casos := []struct {
		nombre string
		envio  Envio
		ahora  string
		quiero bool
	}{
		{"en transito y vencido", Envio{Estado: EnTransito, FechaEntregaEstimada: &eta}, "2026-11-23T18:00:01Z", true},
		{"justo en la fecha no esta demorado", Envio{Estado: EnTransito, FechaEntregaEstimada: &eta}, "2026-11-23T18:00:00Z", false},
		{"antes de la fecha", Envio{Estado: EnDistribucion, FechaEntregaEstimada: &eta}, "2026-11-23T10:00:00Z", false},
		{"entregado nunca esta demorado", Envio{Estado: Entregado, FechaEntregaEstimada: &eta}, "2026-11-30T00:00:00Z", false},
		{"cancelado nunca esta demorado", Envio{Estado: Cancelado, FechaEntregaEstimada: &eta}, "2026-11-30T00:00:00Z", false},
		{"sin fecha estimada", Envio{Estado: EnTransito}, "2026-11-30T00:00:00Z", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.envio.Demorado(ts(t, c.ahora)); got != c.quiero {
				t.Fatalf("Demorado = %v, queria %v", got, c.quiero)
			}
		})
	}
}

func TestEntregadoConDemora(t *testing.T) {
	eta := ts(t, "2026-11-23T18:00:00Z")
	tarde := ts(t, "2026-11-24T09:00:00Z")
	aTiempo := ts(t, "2026-11-23T11:00:00Z")

	if !(Envio{Estado: Entregado, FechaEntregaEstimada: &eta, EntregadoEn: &tarde}).EntregadoConDemora() {
		t.Fatal("entregado despues de la fecha deberia tener la marca")
	}
	if (Envio{Estado: Entregado, FechaEntregaEstimada: &eta, EntregadoEn: &aTiempo}).EntregadoConDemora() {
		t.Fatal("entregado a tiempo no deberia tener la marca")
	}
}
