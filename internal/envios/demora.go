package envios

import "time"

// Envio es el estado actual de un envío, derivado de sus eventos.
type Envio struct {
	ShipmentID           string
	Estado               Estado
	EstadoDesde          time.Time  // timestamp del evento que fijó el estado actual
	FechaEntregaEstimada *time.Time // la última informada por el operador
	ETAInformadaEn       *time.Time // timestamp del evento que trajo esa fecha
	EntregadoEn          *time.Time
	ActualizadoEn        time.Time
}

// Demorado aplica la regla del PO: el envío no está ENTREGADO ni CANCELADO y
// "ahora" ya superó la fecha de entrega estimada. Sin margen de tolerancia.
// Sin fecha estimada no hay contra qué medir, así que no está demorado.
func (e Envio) Demorado(ahora time.Time) bool {
	if e.Estado.Cerrado() || e.FechaEntregaEstimada == nil {
		return false
	}
	return ahora.After(*e.FechaEntregaEstimada)
}

// EntregadoConDemora es la marca que pidió el PO: quedó ENTREGADO, pero la
// entrega ocurrió después de la fecha prometida.
func (e Envio) EntregadoConDemora() bool {
	if e.Estado != Entregado || e.EntregadoEn == nil || e.FechaEntregaEstimada == nil {
		return false
	}
	return e.EntregadoEn.After(*e.FechaEntregaEstimada)
}
