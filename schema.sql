-- Tabla unica append-only: cada corrida que aporta datos nuevos inserta la
-- serie completa con un mismo ingested_at. La "serie ultima" se obtiene
-- desde la vista emae_latest.
CREATE TABLE IF NOT EXISTS emae (
    id                              BIGSERIAL PRIMARY KEY,
    fecha                           DATE             NOT NULL,
    emae                            DOUBLE PRECISION NOT NULL,
    emae_var_anual                  DOUBLE PRECISION,
    emae_desest                     DOUBLE PRECISION,
    emae_desest_var_mensual         DOUBLE PRECISION,
    emae_tendencia_ciclo            DOUBLE PRECISION,
    emae_tendencia_ciclo_var_mensual DOUBLE PRECISION,
    ingested_at                     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emae_fecha_ingested
    ON emae (fecha, ingested_at DESC);

CREATE INDEX IF NOT EXISTS idx_emae_ingested
    ON emae (ingested_at);

-- Vista con la ultima ingesta por fecha. Para cada fecha toma la fila con
-- el ingested_at mas alto.
CREATE OR REPLACE VIEW emae_latest AS
SELECT DISTINCT ON (fecha)
       fecha,
       emae,
       emae_var_anual,
       emae_desest,
       emae_desest_var_mensual,
       emae_tendencia_ciclo,
       emae_tendencia_ciclo_var_mensual,
       ingested_at
FROM emae
ORDER BY fecha, ingested_at DESC;
