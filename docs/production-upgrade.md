# Production upgrade runbook

This runbook treats the production data directory as irreplaceable. Never run a
new build against the only copy of that directory, and never deploy directly
from a dirty working tree.

## 1. Identify the deployed state

Record the current version, executable checksum, service definition, data path,
and free disk space. Keep the exact current binary as a rollback artifact.

```bash
etiquetta version
sha256sum "$(command -v etiquetta)"
sudo systemctl cat etiquetta
sudo systemctl show etiquetta -p Environment -p ExecStart
df -h /var/lib/etiquetta /var/backups
```

Create the release candidate from a clean commit and keep its `checksums.txt`
beside it. Do not build the production candidate from this working directory
while it contains unrelated changes.

## 2. Take an implementation-independent backup

During a maintenance window, stop the service and first archive the untouched
data directory using operating-system tools. This backup does not depend on the
new binary being able to read the old database.

```bash
ETIQUETTA_BACKUP_STAMP=$(date -u +%Y%m%dT%H%M%SZ)
ETIQUETTA_RAW_BACKUP="/var/backups/etiquetta/pre-upgrade-${ETIQUETTA_BACKUP_STAMP}.tar.gz"
sudo systemctl stop etiquetta
sudo install -d -o etiquetta -g etiquetta -m 0700 /var/backups/etiquetta
sudo tar -C /var/lib -czf "$ETIQUETTA_RAW_BACKUP" etiquetta
sudo chmod 0600 "$ETIQUETTA_RAW_BACKUP"
sudo sha256sum "$ETIQUETTA_RAW_BACKUP" | sudo tee "${ETIQUETTA_RAW_BACKUP}.sha256" >/dev/null
```

Keep Etiquetta stopped until every backup command has completed. Copy the
archive and checksum to a separate encrypted system before proceeding.

## 3. Create and validate the logical inventory

Use the release candidate's offline backup command against the stopped data
directory. The command checkpoints DuckDB, refuses a live database, checksums
every file, and records migration/table counts and latest timestamps.

```bash
ETIQUETTA_VERIFIED_BACKUP="/var/backups/etiquetta/verified-${ETIQUETTA_BACKUP_STAMP}.tar.gz"
sudo -u etiquetta /path/to/verified/etiquetta-candidate \
  --data /var/lib/etiquetta backup \
  --output "$ETIQUETTA_VERIFIED_BACKUP"
sudo -u etiquetta sh -c "cd /var/backups/etiquetta && sha256sum -c verified-${ETIQUETTA_BACKUP_STAMP}.tar.gz.sha256"
```

## 4. Rehearse on a copy

Extract a backup into a new staging-only directory. Start the candidate on a
different port with that copied directory, allowing migrations to affect only
the copy. Verify at minimum:

- `/health` and `/api/version` respond;
- an administrator can sign in;
- domains/users and the newest event timestamp match the backup manifest;
- event, consent, replay, error, and performance counts have not decreased;
- tracking, consent choices, and Tag Manager behavior work on a staging site;
- shutdown completes and a second start succeeds.

Do not continue if any invariant differs without a documented explanation.

## 5. Roll out with an explicit rollback point

Keep the maintenance window active, repeat the offline backup if production was
restarted after step 2, atomically install the verified candidate, and retain
the old executable as `etiquetta.previous`. Start the service and repeat the
same smoke checks before reopening traffic.

If the new version fails, stop it. Preserve the post-upgrade directory for
forensics; do not run the old binary against a database already migrated by the
new version. Restore the pre-upgrade archive into a separate directory, point
the old binary at that restored copy, verify its checksum, and only then reopen
traffic.

## Release gate

An upgrade is ready only when the repository tests, race tests, UI build,
installer validation, staging restore rehearsal, and rollback rehearsal all
pass from the exact commit and artifacts intended for production.
