CREATE TABLE IF NOT EXISTS emae (
    id                              SERIAL PRIMARY KEY,
    fecha                           DATE             NOT NULL,
    emae                            DOUBLE PRECISION NOT NULL,
    emae_var_anual                  DOUBLE PRECISION,
    emae_desest                     DOUBLE PRECISION,
    emae_desest_var_mensual         DOUBLE PRECISION,
    emae_tendencia_ciclo            DOUBLE PRECISION,
    emae_tendencia_ciclo_var_mensual DOUBLE PRECISION,
    ingested_at                     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emae_fecha
    ON emae (fecha);

CREATE INDEX IF NOT EXISTS idx_emae_ingested
    ON emae (ingested_at);
