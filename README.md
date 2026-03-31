# EMAE Downloader

Descarga el Estimador Mensual de Actividad Economica (EMAE) publicado por [INDEC](https://www.indec.gob.ar/) e ingesta los datos en una base PostgreSQL.

## Fuente de datos

- **URL:** `https://www.indec.gob.ar/ftp/cuadros/economia/sh_emae_mensual_base2004.xls`
- **Hoja:** primera hoja del archivo XLS
- **Columnas extraidas (C-H):**

| Columna | Descripcion |
|---------|-------------|
| `emae` | EMAE base 2004 |
| `emae_var_anual` | Variacion porcentual interanual |
| `emae_desest` | EMAE desestacionalizado |
| `emae_desest_var_mensual` | Variacion porcentual mensual desestacionalizada |
| `emae_tendencia_ciclo` | Tendencia-ciclo |
| `emae_tendencia_ciclo_var_mensual` | Variacion porcentual mensual tendencia-ciclo |

- **Frecuencia:** mensual, desde enero 2004
- **Fecha:** primer dia del mes correspondiente

## Requisitos

- Go 1.22+
- PostgreSQL

## Configuracion

Variables de entorno requeridas:

```bash
export POSTGRES_USER=...
export POSTGRES_PASSWORD=...
export POSTGRES_HOST=...
export POSTGRES_PORT=5432
export POSTGRES_DB=...
```

## Instalacion

```bash
go build -o emae_downloader .
```

## Creacion del esquema

```bash
psql "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB" -f schema.sql
```

## Uso

**Carga completa (trunca la tabla antes de insertar):**

```bash
./emae_downloader -truncate
```

**Carga incremental (upsert por fecha):**

```bash
./emae_downloader -upsert
```

**Archivo local (sin descarga):**

```bash
./emae_downloader -file /ruta/al/archivo.xls -upsert
```

## Opciones

| Flag | Default | Descripcion |
|------|---------|-------------|
| `-file` | *(descarga)* | Ruta a archivo XLS local |
| `-truncate` | `false` | Truncar tabla antes de insertar |
| `-upsert` | `false` | Usar UPSERT en lugar de COPY |

## Esquema de datos

| Columna | Tipo | Descripcion |
|---------|------|-------------|
| `id` | `SERIAL` | Clave primaria |
| `fecha` | `DATE` | Primer dia del mes |
| `emae` | `DOUBLE PRECISION` | Indice EMAE base 2004 |
| `emae_var_anual` | `DOUBLE PRECISION` | Var. % interanual |
| `emae_desest` | `DOUBLE PRECISION` | EMAE desestacionalizado |
| `emae_desest_var_mensual` | `DOUBLE PRECISION` | Var. % mensual desest. |
| `emae_tendencia_ciclo` | `DOUBLE PRECISION` | Tendencia-ciclo |
| `emae_tendencia_ciclo_var_mensual` | `DOUBLE PRECISION` | Var. % mensual tend.-ciclo |
| `ingested_at` | `TIMESTAMPTZ` | Timestamp de ingesta |

## Consultas de ejemplo

```sql
-- Ultimos 12 meses del EMAE
SELECT fecha, emae, emae_var_anual
FROM emae
ORDER BY fecha DESC
LIMIT 12;

-- EMAE desestacionalizado con variacion mensual
SELECT fecha, emae_desest, emae_desest_var_mensual
FROM emae
WHERE emae_desest IS NOT NULL
ORDER BY fecha DESC
LIMIT 12;
```
