---
layout: default
title: Compose
nav_order: 4
---

# Compose
{: .no_toc }

Compose lets you define and run multi-service unikernel applications from a single YAML file.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## File Format

```yaml
version: "1"

services:
  <service-name>:
    image: <name:tag or file path>
    memory: <256M|1G|...>
    cpus: <number>
    depends_on:
      - <other-service>
    networks:
      - <network-name>
    environment:
      - KEY=VALUE

networks:
  <network-name>:
    driver: bridge
```

---

## Fields Reference

### Top-level

| Field | Required | Description |
|---|---|---|
| `version` | Yes | Must be `"1"` |
| `services` | Yes | Map of service definitions (at least one) |
| `networks` | No | Map of network definitions |

---

### Service fields

| Field | Required | Default | Description |
|---|---|---|---|
| `image` | Yes | — | Image `name:tag` from local store, or a file path to a bootable disk image (`.img`) built with `uni build` |
| `memory` | No | `256M` | VM memory (QEMU format: `256M`, `1G`, `4G`) |
| `cpus` | No | `1` | Number of virtual CPUs |
| `depends_on` | No | `[]` | Services that must start before this one |
| `networks` | No | `[]` | Logical networks to attach to |
| `environment` | No | `[]` | Environment variables as `KEY=VALUE` strings |

---

### Network fields

| Field | Required | Default | Description |
|---|---|---|---|
| `driver` | No | `bridge` | Network driver. Only `bridge` is supported |

---

## Full Example

A web application with a frontend, a backend API, and a database:

```yaml
version: "1"

services:
  db:
    image: postgres:latest
    memory: 512M
    cpus: 1
    networks:
      - backend-net

  api:
    image: myapi:v1.0
    memory: 256M
    cpus: 2
    depends_on:
      - db
    networks:
      - backend-net
      - frontend-net
    environment:
      - DB_HOST=db
      - DB_PORT=5432
      - LOG_LEVEL=info

  web:
    image: myweb:v1.0
    memory: 128M
    cpus: 1
    depends_on:
      - api
    networks:
      - frontend-net
    environment:
      - API_URL=http://api:8080

networks:
  backend-net:
    driver: bridge
  frontend-net:
    driver: bridge
```

**Startup order** (resolved by dependency graph):

```
db  →  api  →  web
```

**Shutdown order** (always reversed):

```
web  →  api  →  db
```

---

## How Dependency Ordering Works

Uni uses **Kahn's topological sort** algorithm to determine startup order:

1. Build a dependency graph from all `depends_on` declarations
2. Start services with no dependencies first
3. When a service finishes starting, unlock any services that depended on it

If a **dependency cycle** is detected (e.g. A depends on B, B depends on A), `uni compose up` will fail immediately:

```
Error: compose up: compose: dependency cycle detected
```

---

## State File

When you run `uni compose up stack.yaml`, a state file is created in the same directory:

```
stack.yaml
.uni-compose-state.json   ← automatically created
```

Content:
```json
{
  "project": "myproject",
  "services": {
    "db":  "a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f",
    "api": "b4e9d3e2-8c5f-5b2g-9d3e-2b3c4d5e6f7a",
    "web": "c5f0e4f3-9d6a-6c3h-ae4f-3c4d5e6f7a8b"
  }
}
```

Commands `down`, `ps`, and `logs` read this file to know which VM IDs belong to the stack.

{: .warning }
Do not delete `.uni-compose-state.json` manually while the stack is running. If it gets lost, use `uni ps` to find the VM IDs and stop them individually with `uni stop`.

---

## Minimal Example

The simplest possible compose file — one service, no networks.

Build the image first, then reference it by name:

```bash
uni build ./hello-linux --name hello
```

```yaml
version: "1"
services:
  hello:
    image: hello:latest
    memory: 256M
```

```bash
uni compose up hello.yaml
# started hello → a3f8c2d1-...

uni compose ps hello.yaml
# SERVICE  ID              STATE
# hello    a3f8c2d1-...    running

uni compose logs hello.yaml hello
# Hello from unikernel!

uni compose down hello.yaml
# stopped hello
```
