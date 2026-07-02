# MySQL on Jerboa

This builds a unikernel that boots MySQL and accepts connections on port 3306,
using the `eyberg/mysql:5.7.29` package pulled from the `ops` ecosystem.

## Why there is no init step

The `eyberg/mysql` package ships a **pre-initialized data directory** baked
into the image (`sysroot/var/lib/mysql`, containing the `mysql` system
database, `ibdata1`, certs, …), so the server can start against it directly —
no `mysqld --initialize`/`--initialize-insecure` step is needed. Unlike
PostgreSQL's `initdb`, MySQL's own initialize step doesn't fork/exec helper
processes, so it could in principle run inside a unikernel too — but since the
package already ships the result, there's no reason to.

## Build and run

```sh
jerboa network create mynet
jerboa build . --name mysql
jerboa run mysql --network mynet -p 3306:3306
jerboa logs <id>     # → "mysqld: ready for connections"
```

The package (`eyberg/mysql:5.7.29`) and its source (`ops`) are declared in
`unikernel.toml` under `[build] pkgs` / `pkg_source`, so the build needs no
`--pkg` flags. Add `--smoke` to boot-test the image right after building.

`memory` (`512M`) and the published port (`3306:3306`) come from `[run]` in
`unikernel.toml` and are inherited automatically; the port publishes because
the VM joined `mynet`. Any explicit flag (`--memory`, `-p`) still overrides
the baked default.

## How the `unikernel.toml` works

- `lang = "raw"` — no compilation; the binaries come from the declared package.
- `pkgs = ["eyberg/mysql:5.7.29"]` / `pkg_source = "ops"` — the package and
  where it comes from, baked into the config so builds are reproducible without
  flags.
- `[program] path = "mysqld"` — the package ships a single `mysqld` binary at
  the image root, so the bare basename resolves unambiguously (no same-named
  stub to disambiguate from, unlike `postgres`).
- `[program] args = ["--datadir=/var/lib/mysql", ...]` — run against the
  pre-seeded data directory.
- `disk_size = "512M"` — reserves scratch space inside the root image for
  logs, sockets, and the database files.

## Persistence (seeded volume)

`/var/lib/mysql` baked into the image is **ephemeral** — removing the VM
(`jerboa rm`) discards the data. To make the database durable, put the data
directory on a TFS volume. A freshly created volume is empty, and mounting an
empty volume over `/var/lib/mysql` would just shadow the seeded data — so the
volume has to be **seeded once** with the initialized data directory before
it is mounted. `jerboa volume seed` does this: it writes a package subtree
into the volume's filesystem with `mkfs`.

```sh
# 1. Create the volume and seed it with the package's pre-initialized data
#    directory in one step (/var/lib/mysql → volume root)
jerboa volume create mysqldata --size 512M \
  --seed-pkg eyberg/mysql:5.7.29 --src /var/lib/mysql

# 2. Run with the volume mounted at /var/lib/mysql (it shadows the baked copy)
jerboa network create mynet
jerboa run mysql -v mysqldata:/var/lib/mysql --network mynet -p 3306:3306
```

(`jerboa volume seed` still exists for re-seeding an existing volume — it
reformats and resets it to the seed state.)

Now the data survives recreating the VM:

```sh
jerboa stop <id>          # graceful shutdown
jerboa rm <id>             # remove the VM entirely
jerboa run mysql -v mysqldata:/var/lib/mysql --network mynet -p 3306:3306
```

Wipe the data with `jerboa volume rm mysqldata` (and re-seed to start fresh).

## Notes

- `mysqld` is configured with `--bind-address=0.0.0.0` so it accepts
  connections on the managed-network interface, not just localhost.
- See `../postgresql/README.md` for the general volume-seeding model (TFS
  label matching, QEMU `opt/uni/mounts` fw_cfg vs. Firecracker boot args) and
  `../mongodb/README.md` for a database that needs no seeding at all.
