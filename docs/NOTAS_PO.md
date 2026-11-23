# Notas de la charla con Mario (PO Envíos y Logística)

## Integración con el operador
- Las novedades llegan por **webhook**: el operador nos hace un `POST` por cada novedad.
- Payload JSON:
  - `shipment_id` (string)
  - `status` (string): `CREADO`, `EN_TRANSITO`, `EN_DISTRIBUCION`, `ENTREGADO`, `CANCELADO`
  - `timestamp` (ISO 8601): cuándo ocurrió el evento
  - `estimated_delivery_date` (ISO 8601, opcional): suele venir en el primer evento o cuando se pacta la entrega

## Dudas abiertas
- Qué cuenta como "demorado".
- Reintentos, duplicados y eventos fuera de orden.
- Transiciones válidas.
