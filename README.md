# logistica-eventos-sla

Servicio en Go que recibe las novedades de envíos que manda el operador logístico por webhook,
guarda el historial completo, mantiene el estado actual de cada envío y expone consultas de
seguimiento con la alerta de demora.

Las reglas de negocio salen de la charla con el PO (Mario). Están anotadas en
[`docs/NOTAS_PO.md`](docs/NOTAS_PO.md), y los supuestos que tomé están más abajo.

## Cómo correrlo

Requiere Go 1.22 o superior. La base es SQLite con un driver puro Go (`modernc.org/sqlite`), así
que no hace falta cgo ni levantar nada aparte.

```bash
go run ./cmd/servidor              # escucha en :8080 y crea envios.db
PORT=9000 DB_PATH=/tmp/e.db go run ./cmd/servidor
go test ./...
```

Hay ejemplos listos para probar a mano en [`http/ejemplos.http`](http/ejemplos.http).

## Endpoints

| Método | Ruta | Qué hace |
|---|---|---|
| `POST` | `/webhooks/operador/eventos` | Recibe un evento del operador |
| `GET` | `/envios/{shipment_id}` | Estado actual, con `demorado` y `entregado_con_demora` |
| `GET` | `/envios/{shipment_id}/eventos` | Historial en orden cronológico (cuándo ocurrió cada evento) |
| `GET` | `/health` | Chequeo de vida |

### Webhook

```json
{
  "event_id": "evt-001",
  "shipment_id": "SHP-123",
  "status": "EN_TRANSITO",
  "timestamp": "2026-11-23T08:00:00-03:00",
  "estimated_delivery_date": "2026-11-25T18:00:00-03:00"
}
```

| Respuesta | Cuándo |
|---|---|
| `200` `{"resultado":"aplicado"}` | Evento nuevo que cambió el estado actual |
| `200` `{"resultado":"solo_historial"}` | Evento nuevo pero más viejo que el estado actual: queda en el historial, no pisa el estado |
| `200` `{"resultado":"duplicado"}` | Ese `event_id` ya estaba: no se guarda de nuevo |
| `400` | Payload inválido: JSON roto, faltan campos, `status` desconocido o fecha sin zona horaria. No tiene sentido reintentarlo |
| `500` | Error nuestro. El operador puede reintentar: el `event_id` evita duplicar |

### Estado actual

```json
{
  "shipment_id": "SHP-123",
  "estado": "EN_TRANSITO",
  "estado_desde": "2026-11-23T11:00:00Z",
  "fecha_entrega_estimada": "2026-11-25T21:00:00Z",
  "entregado_en": null,
  "demorado": true,
  "entregado_con_demora": false
}
```

## Reglas de negocio (confirmadas con el PO)

- **Demorado**: el estado no es `ENTREGADO` ni `CANCELADO` y la hora actual ya superó la
  `estimated_delivery_date`. Sin margen de tolerancia. Se calcula **al consultar**, no se guarda:
  si el operador deja de mandar eventos, el envío igual pasa a demorado cuando vence.
- **Entregado con demora**: el envío queda `ENTREGADO`, pero con la marca si el evento de entrega
  ocurrió después de la fecha prometida.
- **Duplicados**: el operador reenvía. El `event_id` es la clave primaria del historial; un
  reenvío responde `200` y no se guarda dos veces.
- **Fuera de orden**: el estado actual es siempre el del evento con `timestamp` más reciente. Uno
  más viejo que llega tarde queda en el historial y no pisa el estado.
- **Envío desconocido**: si llega primero un evento que no es `CREADO`, el envío se crea con lo
  que traiga.
- **Historial**: ordenado por cuándo ocurrió cada evento, no por cuándo llegó.
- **Sin autenticación** en el webhook para este MVP, por pedido del PO.

## Supuestos míos (a confirmar)

1. **Fecha estimada re-pactada**: vale la última informada según el `timestamp` del evento que la
   trae; una fecha vieja que llega tarde no pisa la nueva. Efecto de negocio que le planteé al PO:
   si se re-pacta después de vencida, el envío deja de figurar demorado. Mario tiene que
   confirmarlo con soporte y operaciones (impacta en compensaciones). Si hubiera que medir contra
   la primera fecha prometida, es guardar `fecha_entrega_prometida` aparte y no pisarla nunca.
2. **`status` desconocido → 400**. No hay regla formal de transiciones; el PO prefirió no armar
   una máquina de estados. Si operaciones confirma que `ENTREGADO` y `CANCELADO` son finales, se
   agrega en `envios.Aplicar`.
3. **Timestamps iguales**: si llega un evento con el mismo `timestamp` que el estado actual, se
   conserva el que estaba, para que el resultado no dependa del orden de llegada.
4. **Fechas**: el operador manda ISO 8601 con zona horaria (confirmado). Se guardan en UTC; una
   fecha sin zona se rechaza con 400 en vez de suponer la hora de Argentina.
5. **Sin fecha estimada** no hay contra qué medir: el envío no figura demorado.

## Diseño

```
cmd/servidor        arranque, configuración por variables de entorno, apagado prolijo
internal/envios     dominio puro: estados, Aplicar (evento → estado nuevo), reglas de demora
internal/almacen    SQLite: historial (envio_evento) y estado actual (envio)
internal/api        HTTP: webhook y consultas
```

- `envios.Aplicar` es una función pura: toda la lógica de orden, duplicados y fecha estimada se
  testea sin base.
- Registrar un evento es **una transacción**: insertar en el historial (`ON CONFLICT DO NOTHING`
  sobre `event_id`), leer el estado actual, aplicar y guardar. O quedan las dos cosas o ninguna.
- SQLite con una sola conexión serializa las escrituras, así que dos eventos del mismo envío no se
  pisan. **En PostgreSQL** cambiaría el lock: `SELECT ... FOR UPDATE` sobre la fila del envío
  dentro de la misma transacción.
- Se guarda el estado actual en una tabla aparte para que la consulta no recorra el historial.

## Tests

```bash
go test ./...
```

- `internal/envios`: reglas de demora (incluido el borde "justo en la fecha") y `Aplicar`
  (fuera de orden, timestamps iguales, envío nuevo sin `CREADO`, fecha vieja que no pisa la
  re-pactada).
- `internal/api`: la API real contra SQLite en memoria: duplicado, fuera de orden, demorado contra
  la hora actual, entregado con demora, validaciones y 404.

## Lo que dejé afuera (acordado)

- Listado de envíos demorados y notificaciones activas: el PO dijo que alcanza con el GET.
- Autenticación del webhook.
- Cuánto tiempo lleva de demora.

## Próximos pasos

- Confirmar con operaciones los supuestos 1 y 2.
- Firma HMAC del webhook cuando el operador la soporte.
- Migraciones versionadas y PostgreSQL para producción.
