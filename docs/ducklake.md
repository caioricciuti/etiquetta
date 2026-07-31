# DuckLake Storage

Etiquetta stores its high-write event data in a [DuckLake](https://ducklake.select)
catalog. This is the **default backend** (`ETIQUETTA_STORAGE=ducklake`) for new
and existing installs.

To keep everything in the single DuckDB file instead, set
`ETIQUETTA_STORAGE=duckdb`. On an existing DuckDB install, leaving the variable
unset switches it to DuckLake on the next start — a safe, one-way migration that
snapshots the DuckDB file first (see [Enabling](#enabling)).

## What changes

| | `duckdb` | Default (`ducklake`) |
| --- | --- | --- |
| `events`, `performance`, `errors` | tables in `etiquetta.duckdb` | Parquet files + catalog under `{data-dir}/lake` |
| metadata (`domains`, `users`, `settings`, …) | `etiquetta.duckdb` | `etiquetta.duckdb` (unchanged) |
| `visitor_sessions` | `etiquetta.duckdb` | `etiquetta.duckdb` (rebuilt via DELETE+INSERT, not lake-suitable) |
| ingestion | in-memory buffer → Parquet file → `INSERT` | same buffer, loads into the lake table |
| history | none | snapshots + `AT (VERSION => n)` time-travel |
| compaction | DuckDB space reclaim | `ducklake_merge_adjacent_files` + snapshot expiry |

## How it works

DuckLake does not support `PRIMARY KEY`/`UNIQUE` constraints, and the metadata
tables rely on them — so Etiquetta uses a hybrid layout rather than moving
everything:

1. On startup every pooled connection runs a boot hook that loads the `ducklake`,
   `httpfs`, and `icu` extensions, attaches the lake as `lake`, and sets
   `search_path='<db>.main,lake.main'` (main first).
2. Migrations run normally. `CREATE TABLE` targets the first search-path entry —
   the main database — so metadata keeps its constraints.
3. `Lakeify` copies `events`/`performance`/`errors` into the lake (widening
   `page_duration` to `BIGINT`), verifies the row counts, then **drops** them
   from the main database.
4. Because those tables no longer exist in main, unqualified `events` (and its
   `INSERT`/`UPDATE`/`DELETE`) fall through the search path to the mutable lake
   table. No views are used — views are read-only and would break retention
   deletes and bot-score updates.

The result: every existing query works unchanged, ingestion writes to the lake,
and the `page_duration` INT32 overflow is structurally impossible (BIGINT).

## Enabling

DuckLake is the default, so a fresh install needs no configuration. When an
existing DuckDB install first starts on DuckLake, the transition is a one-way
migration (event tables are moved into the lake and dropped from the DuckDB
file). Etiquetta writes a rollback snapshot to
`{data-dir}/etiquetta.duckdb.pre-ducklake` before doing so.

```bash
etiquetta backup --output ./etiquetta-backup.tar.zst   # recommended for upgrades
etiquetta stop
etiquetta serve                                         # DuckLake is the default
```

Evaluate first without switching the live server — build a lake copy alongside
the current database (read-only, non-destructive):

```bash
etiquetta ducklake-migrate --data ./data
```

## Rolling back

```bash
etiquetta stop
mv {data-dir}/etiquetta.duckdb.pre-ducklake {data-dir}/etiquetta.duckdb
rm -rf {data-dir}/lake
# set ETIQUETTA_STORAGE=duckdb (so it doesn't migrate again), then start
```

## Backups

Back up **both** `{data-dir}/etiquetta.duckdb` and `{data-dir}/lake/` — the event
data lives in the lake directory (Parquet files + `catalog.ducklake`).

## Notes

- Extensions are installed on first run and need network access at that point.
- The lake catalog stores absolute paths; keep `{data-dir}` stable, or re-run the
  transition after moving it.
