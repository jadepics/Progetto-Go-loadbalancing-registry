
Implementazione “Service Registry + client-side load balancing” come compito del corso di Sistemi Distribuiti e Cloud Computing 2025/2026

Componenti

 Registry (`cmd/registry`)
Espone metodi RPC:

- `Registry.Register` — registrazione di un’istanza di servizio
- `Registry.Deregister` — deregistrazione su shutdown
- `Registry.Lookup` — lista istanze attive per un servizio

Il registry mantiene uno stato in-memory delle istanze registrate.

Servizi RPC stateless

- **Echo** (`cmd/echo`): `Echo.Echo(msg) -> msg`
- **Math** (`cmd/math`): `Math.Add(a,b) -> sum`

All’avvio si registrano nel registry; su `SIGTERM/SIGINT` si deregistrano.

 Servizio RPC stateful (Primary/Backup) — KV (`cmd/kv`)
> Questa sezione richiede che nel repo esistano `cmd/kv` e i tipi RPC in `common/kv.go`.

Servizio key-value replicato con consistenza forte (schema **Primary/Backup**):

- **Primary**
  - accetta `KV.Put(key,value)`
  - applica localmente l’update
  - replica in modo sincrono ai backup tramite `KV.Apply(seq,key,value)`
- **Backup**
  - serve le letture `KV.Get(key)`
  - se riceve un `KV.Put` da un client, risponde con `OK=false` e `RedirectTo=<addr primary>`

Bootstrap : il backup può inizializzare lo stato via `KV.Snapshot()` chiamato al primary.

### Client (`cmd/client`)

- Fa `Registry.Lookup(service)` **una sola volta** all’inizio della sessione (**cache locale**)
- Invia `N` richieste in sequenza (simulazione carico)
- Algoritmi di load balancing:
  - `random` (stateless)
  - `rr` (round robin)
  - `wrr` (smooth weighted round robin)



## Esecuzione locale (senza Docker)


### Demo stateless: Echo (2 istanze + client)

**Terminale 1 — Registry**
```bash
go run ./cmd/registry -listen :9000
```

**Terminale 2 — Echo #1**
```bash
go run ./cmd/echo -listen :9101 -public localhost:9101 -registry localhost:9000 -id echo1 -weight 5
```

**Terminale 3 — Echo #2**
```bash
go run ./cmd/echo -listen :9102 -public localhost:9102 -registry localhost:9000 -id echo2 -weight 1
```

**Terminale 4 — Client**
```bash
go run ./cmd/client -registry localhost:9000 -service echo -algo wrr -n 20
```

### Demo stateless: Math
Avvia 1 o più istanze di Math su porte diverse e poi:
```bash
go run ./cmd/client -registry localhost:9000 -service math -algo rr -n 20
```

### Demo stateful: KV Primary/Backup (2 istanze + client)

**Terminale 1 — Registry**
```bash
go run ./cmd/registry -listen :9000
```

**Terminale 2 — KV Primary (kv1)**
```bash
go run ./cmd/kv -listen :9301 -public localhost:9301 -registry localhost:9000 -id kv1 -primary-id kv1
```

**Terminale 3 — KV Backup (kv2)**
```bash
go run ./cmd/kv -listen :9302 -public localhost:9302 -registry localhost:9000 -id kv2 -primary-id kv1
```

**Terminale 4 — Client (PUT: scritture)**
```bash
go run ./cmd/client -registry localhost:9000 -service kv -algo rr -op put -key x -value v -n 10
```

**Terminale 4 — Client (GET: letture bilanciate)**
```bash
go run ./cmd/client -registry localhost:9000 -service kv -algo rr -op get -key x -n 10
```

---

## Docker Compose


### Demo stateless (Echo)

Non avviare tutto insieme: avvia a mano per dimostrare che la lista del registry varia nel tempo.

1) Avvia solo registry:
```bash
docker compose up -d registry
```

2) Avvia una istanza:
```bash
docker compose up -d echo1
```

3) Esegui una sessione client:
```bash
docker compose --profile client up --abort-on-container-exit client
```

4) Avvia un’altra istanza:
```bash
docker compose up -d echo2
```

5) Nuova sessione client (nuovo lookup):
```bash
docker compose --profile client up --abort-on-container-exit client
```

6) Spegni un server (deregistrazione su SIGTERM):
```bash
docker compose stop echo1
```

### Demo stateful (KV Primary/Backup)


1) Avvia registry + primary:
```bash
docker compose up -d --build registry kv1
```

2) Esegui PUT (il client potrebbe colpire un backup solo quando esiste; in questa fase c’è solo primary):
```bash
docker compose --profile client up --build --abort-on-container-exit client
```

3) Avvia un backup:
```bash
docker compose up -d --build kv2
```

4) Esegui GET bilanciati (dovresti vedere risposte da `kv1` e `kv2`):
```bash
docker compose --profile client up --build --abort-on-container-exit client
```



