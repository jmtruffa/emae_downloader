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

-- Tabla append-only para el EMAE por sector de actividad. Cada snapshot
-- inserta la serie completa con indices base 2004 de la primera hoja del XLS.
CREATE TABLE IF NOT EXISTS emae_actividad (
    id                                                           BIGSERIAL PRIMARY KEY,
    fecha                                                        DATE             NOT NULL,
    agricultura_ganaderia_caza_silvicultura                     DOUBLE PRECISION,
    pesca                                                        DOUBLE PRECISION,
    explotacion_minas_canteras                                  DOUBLE PRECISION,
    industria_manufacturera                                      DOUBLE PRECISION,
    electricidad_gas_agua                                        DOUBLE PRECISION,
    construccion                                                 DOUBLE PRECISION,
    comercio_mayorista_minorista_reparaciones                   DOUBLE PRECISION,
    hoteles_restaurantes                                         DOUBLE PRECISION,
    transporte_comunicaciones                                    DOUBLE PRECISION,
    intermediacion_financiera                                    DOUBLE PRECISION,
    actividades_inmobiliarias_empresariales_alquiler             DOUBLE PRECISION,
    administracion_publica_defensa_seguridad_social              DOUBLE PRECISION,
    ensenanza                                                    DOUBLE PRECISION,
    servicios_sociales_salud                                     DOUBLE PRECISION,
    otras_actividades_servicios_comunitarios_sociales_personales DOUBLE PRECISION,
    impuestos_netos_subsidios                                    DOUBLE PRECISION,
    ingested_at                                                  TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emae_actividad_fecha_ingested
    ON emae_actividad (fecha, ingested_at DESC);

CREATE INDEX IF NOT EXISTS idx_emae_actividad_ingested
    ON emae_actividad (ingested_at);

CREATE OR REPLACE VIEW emae_actividad_latest AS
SELECT DISTINCT ON (fecha)
       fecha,
       agricultura_ganaderia_caza_silvicultura,
       pesca,
       explotacion_minas_canteras,
       industria_manufacturera,
       electricidad_gas_agua,
       construccion,
       comercio_mayorista_minorista_reparaciones,
       hoteles_restaurantes,
       transporte_comunicaciones,
       intermediacion_financiera,
       actividades_inmobiliarias_empresariales_alquiler,
       administracion_publica_defensa_seguridad_social,
       ensenanza,
       servicios_sociales_salud,
       otras_actividades_servicios_comunitarios_sociales_personales,
       impuestos_netos_subsidios,
       ingested_at
FROM emae_actividad
ORDER BY fecha, ingested_at DESC;
