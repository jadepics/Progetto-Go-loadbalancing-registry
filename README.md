# Service Registry + Client-side Load Balancing (Go, net/rpc)

Implementazione del requisito “Service Registry + client-side load balancing” (SR + lookup + cache + algoritmi).

## Componenti

- **Registry** (`cmd/registry`): metodi RPC
  - `Registry.Register` (registrazione server)
  - `Registry.Deregister` (deregistrazione server)
  - `Registry.Lookup` (lista istanze attive per servizio)

- **Servizi RPC**
  - **Echo** (`cmd/echo`): `Echo.Echo(msg) -> msg`
  - **Math** (`cmd/math`): `Math.Add(a,b) -> sum`
  - All’avvio registrano se stessi sul registry; su SIGTERM/SIGINT eseguono deregistrazione.

- **Client** (`cmd/client`)
  - Fa `Lookup` **una sola volta all’inizio della sessione** (cache locale)
  - Simula un carico con N richieste
  - Algoritmi:
    - `random` (stateless)
    - `rr` (round robin)
    - `wrr` (smooth weighted round robin, **stateful** sui pesi restituiti dal registry)

## Esecuzione locale (senza Docker)

Terminale 1:
```bash
go run ./cmd/registry -listen :9000
```

Terminale 2:
```bash
go run ./cmd/echo -listen :9101 -public localhost:9101 -registry localhost:9000 -id echo1 -weight 5
```

Terminale 3:
```bash
go run ./cmd/echo -listen :9102 -public localhost:9102 -registry localhost:9000 -id echo2 -weight 1
```

Terminale 4:
```bash
go run ./cmd/client -registry localhost:9000 -service echo -algo wrr -n 20
```

## Docker Compose: simulazione dinamica

Non avviare tutto insieme: avvia a mano per dimostrare che la lista del registry varia nel tempo.

1) Avvia **solo** registry:
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
