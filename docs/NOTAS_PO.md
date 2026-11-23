# Notas de la charla con Mario (PO Envíos y Logística)

## Integración con el operador
- Las novedades llegan por **webhook**: el operador nos hace un `POST` por cada novedad.
- Payload JSON:
  - `shipment_id` (string)
  - `status` (string): `CREADO`, `EN_TRANSITO`, `EN_DISTRIBUCION`, `ENTREGADO`, `CANCELADO`
  - `timestamp` (ISO 8601): cuándo ocurrió el evento
  - `estimated_delivery_date` (ISO 8601, opcional): suele venir en el primer evento o cuando se pacta la entrega

## Regla de demora (confirmada)
- **Demorado**: el estado actual no es `ENTREGADO` ni `CANCELADO` y "ahora" ya superó `estimated_delivery_date`. Sin margen de tolerancia.
- **Entregado con demora**: el estado queda `ENTREGADO`, pero con una marca si la entrega (timestamp del evento `ENTREGADO`) fue posterior a la fecha prometida.
- Si la fecha se re-pacta: Mario no lo sabe; se usa "la fecha estimada que tengamos asociada al envío". Decisión mía: me quedo con la última informada (ver README).

## Reintentos, duplicados y orden (confirmado)
- Cada evento trae un `event_id` único (no estaba en el primer ejemplo).
- El operador **reenvía** eventos y **no garantiza el orden**.
- Duplicado exacto (mismo `event_id`): responder 200/204 y no duplicar en la base.
- Evento con `timestamp` anterior al último procesado: **no** pisa el estado actual, pero **sí** queda en el historial.

## Estados y transiciones
- No hay regla formal de transiciones (Mario tiene que chequearlo con operaciones).
- Para el MVP: **el estado actual es el del evento con timestamp más reciente**, sin máquina de estados.
- Status fuera de los 5 conocidos: lo rechazo con 400 (supuesto mío, avisado al PO).
- Evento de un `shipment_id` nunca visto, aunque no sea `CREADO`: se acepta y se crea el envío.

## Webhook
- Sin autenticación para el MVP (pedido explícito del PO: foco en dominio y datos).

## Consultas
- Historial ordenado por `timestamp` del evento (cuándo ocurrió), no por llegada.
- Alertas: alcanza con que el GET del envío diga `demorado` y `entregado_con_demora`. Sin listado de demorados ni notificaciones activas.

## Dudas abiertas
