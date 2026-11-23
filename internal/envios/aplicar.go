package envios

import "time"

// Aplicar calcula el nuevo estado actual de un envío a partir del evento que
// llega. Es una función pura: no toca la base, así se testea sin infraestructura.
//
// Reglas acordadas con el PO:
//   - El estado actual refleja siempre el evento con el timestamp más reciente.
//     Un evento más viejo que llega tarde (la red no garantiza el orden) no lo
//     pisa, aunque sí queda en el historial (eso lo hace el repositorio).
//   - Si el envío no existía, se crea con lo que traiga el evento, aunque no sea
//     CREADO: puede llegar antes un evento posterior.
//   - La fecha estimada vigente es la última informada según el timestamp del
//     evento, con el mismo criterio: una fecha vieja no pisa una re-pactada.
//
// Con timestamps iguales se queda el estado que ya estaba: no hay forma de
// saber cuál fue primero y así el resultado no depende del orden de llegada.
func Aplicar(actual *Envio, ev Evento, ahora time.Time) Envio {
	if actual == nil {
		nuevo := Envio{
			ShipmentID:    ev.ShipmentID,
			Estado:        ev.Estado,
			EstadoDesde:   ev.OcurridoEn,
			ActualizadoEn: ahora,
		}
		if ev.FechaEntregaEstimada != nil {
			nuevo.FechaEntregaEstimada = ev.FechaEntregaEstimada
			ocurrido := ev.OcurridoEn
			nuevo.ETAInformadaEn = &ocurrido
		}
		if ev.Estado == Entregado {
			ocurrido := ev.OcurridoEn
			nuevo.EntregadoEn = &ocurrido
		}
		return nuevo
	}

	sig := *actual
	sig.ActualizadoEn = ahora

	if ev.OcurridoEn.After(actual.EstadoDesde) {
		sig.Estado = ev.Estado
		sig.EstadoDesde = ev.OcurridoEn
		if ev.Estado == Entregado {
			ocurrido := ev.OcurridoEn
			sig.EntregadoEn = &ocurrido
		} else {
			sig.EntregadoEn = nil
		}
	}

	if ev.FechaEntregaEstimada != nil &&
		(actual.ETAInformadaEn == nil || !ev.OcurridoEn.Before(*actual.ETAInformadaEn)) {
		sig.FechaEntregaEstimada = ev.FechaEntregaEstimada
		ocurrido := ev.OcurridoEn
		sig.ETAInformadaEn = &ocurrido
	}

	return sig
}
