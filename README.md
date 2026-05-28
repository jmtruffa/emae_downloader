# EMAE Downloader

Descarga el Estimador Mensual de Actividad Economica (EMAE) publicado por [INDEC](https://www.indec.gob.ar/) e ingesta los datos en una base PostgreSQL como snapshots append-only.

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

Tambien descarga e ingesta el EMAE por sector de actividad:

- **URL:** `https://www.indec.gob.ar/ftp/cuadros/economia/sh_emae_actividad_base2004.xls`
- **Hoja 1:** indices base 2004 por sector
- **Tabla destino:** `emae_actividad`

## Modelo

Tablas `emae` y `emae_actividad` append-only. Cada corrida que aporta un mes nuevo
inserta la serie completa con un `ingested_at` comun. Para revisiones de
INDEC hacia atras: el supuesto es que solo tocan valores viejos cuando
publican uno nuevo, por lo que basta con detectar el avance de `max(fecha)`
para decidir si vale la pena guardar otro snapshot.

Las vistas `emae_latest` y `emae_actividad_latest` resuelven "la serie ultima"
(para cada `fecha`, la fila con el `ingested_at` mas alto).

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

**Corrida normal (la pensada para un cron diario):**

```bash
./emae_downloader
```

La app:

1. Descarga el XLS de INDEC.
2. Parsea la serie completa.
3. Compara `max(fecha)` parseado contra el `max(fecha)` del ultimo snapshot
   en `emae`.
4. Si el parseado no avanza, **no inserta nada** y termina.
5. Si avanza (o la tabla esta vacia), inserta la serie entera con un
   `ingested_at` comun.

Esto permite ejecutar el cron del 20 al 31 de cada mes sin riesgo de
popular la tabla con snapshots redundantes.

**Forzar la insercion aunque no haya fecha nueva:**

```bash
./emae_downloader -force
```

Util si se quiere registrar explicitamente una revision de INDEC sobre
fechas viejas sin avance del ultimo mes.

**Archivo local (sin descarga):**

```bash
./emae_downloader -file /ruta/al/archivo.xls
```

**Archivo local para actividad (sin descargar la segunda planilla):**

```bash
./emae_downloader -actividad-file /ruta/al/actividad.xls
```

## Opciones

| Flag | Default | Descripcion |
|------|---------|-------------|
| `-file` | *(descarga)* | Ruta a archivo XLS local |
| `-actividad-file` | *(descarga)* | Ruta a archivo XLS local de actividad |
| `-force` | `false` | Insertar snapshot aunque `max(fecha)` no avance |

## Esquema de datos

Tabla `emae`:

| Columna | Tipo | Descripcion |
|---------|------|-------------|
| `id` | `BIGSERIAL` | Clave primaria |
| `fecha` | `DATE` | Primer dia del mes |
| `emae` | `DOUBLE PRECISION` | Indice EMAE base 2004 |
| `emae_var_anual` | `DOUBLE PRECISION` | Var. % interanual |
| `emae_desest` | `DOUBLE PRECISION` | EMAE desestacionalizado |
| `emae_desest_var_mensual` | `DOUBLE PRECISION` | Var. % mensual desest. |
| `emae_tendencia_ciclo` | `DOUBLE PRECISION` | Tendencia-ciclo |
| `emae_tendencia_ciclo_var_mensual` | `DOUBLE PRECISION` | Var. % mensual tend.-ciclo |
| `ingested_at` | `TIMESTAMPTZ` | Timestamp del snapshot |

Indice `(fecha, ingested_at DESC)` para la vista `emae_latest`.

No hay unico sobre `fecha`: se permiten multiples filas por fecha, una por
snapshot.

## Consultas de ejemplo

```sql
-- Serie ultima (equivalente a "la tabla emae de siempre")
SELECT * FROM emae_latest ORDER BY fecha DESC LIMIT 12;

-- Cuantos snapshots tenemos y cuando
SELECT ingested_at, COUNT(*) AS filas, MAX(fecha) AS ultimo_mes
FROM emae
GROUP BY ingested_at
ORDER BY ingested_at;

-- Como INDEC fue revisando un mes a lo largo del tiempo
SELECT ingested_at, emae, emae_desest
FROM emae
WHERE fecha = '2024-01-01'
ORDER BY ingested_at;
```

## Notas

- **Fix de fechas (2026-05):** versiones anteriores avanzaban el contador
  de fecha en filas saltadas del XLS, corriendo toda la serie un mes hacia
  adelante (enero-2004 quedaba registrado como 2004-02-01). El parser ahora
  solo avanza la fecha cuando consume una fila valida.
