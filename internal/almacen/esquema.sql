-- Historial completo: todo evento que llega queda aca, tambien los que llegan
-- tarde y no cambian el estado. event_id es la clave: un reenvio del operador
-- choca contra ella y no se duplica.
CREATE TABLE IF NOT EXISTS envio_evento (
    event_id               TEXT PRIMARY KEY,
    shipment_id            TEXT NOT NULL,
    estado                 TEXT NOT NULL,
    ocurrido_en            TEXT NOT NULL, -- RFC 3339 en UTC
    fecha_entrega_estimada TEXT,
    recibido_en            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_envio_evento_shipment
    ON envio_evento (shipment_id, ocurrido_en);

-- Estado actual derivado, para no recalcularlo en cada consulta.
CREATE TABLE IF NOT EXISTS envio (
    shipment_id            TEXT PRIMARY KEY,
    estado                 TEXT NOT NULL,
    estado_desde           TEXT NOT NULL,
    fecha_entrega_estimada TEXT,
    eta_informada_en       TEXT,
    entregado_en           TEXT,
    actualizado_en         TEXT NOT NULL
);
