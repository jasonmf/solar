# SolarEdge Energy Logging

Retrieve energy productin data from the SolarEdge API, submit it to InfluxDB,
and graph that data with Grafana. This setup is particular to me but could be
adapted to how you do things.

This documentation is incomplete.

## Technology Stack

I run my setup on an Internet-facing VPS. I run each component in a distinct
docker container and orchestrate them clumsily with `docker-compose`. An edited
version of the `docker-compose.yml` is included here. I tag my container images
with `live` to indicate a last-known-good version.

On my server I have `/var/containers/`. That's where I keep the
`docker-compose.yml` and containers that need peristent storage get directories
underneath `/var/containers/`.

The HTTP services aren't directly Internet-facing: they're behind a fork of the
`slt` reverse proxy. `slt` can terminate TLS and forward requests to HTTP
backends. My fork allows `slt` to automatically get certs issued from Let's
Encrypt.

### InfluxDB

I have no idea if I even modified the config. I have `http` enabled and `https`
disabled: the latter is handled by `slt`.

Within influx I approximatey did:

```
CREATE DATABASE "solar"
CREATE USER "solar" WITH PASSWORD "<random password>"
GRANT ALL ON "solar" TO "solar"
CREATE USER "grafana" WITH PASSWORD "<random password>"
GRANT READ ON "solar" TO "grafana"
```

### Grafana

My grafana config looks like the one below. I have Google Workspace account and
I do OAuth through that for auth. You might just want local users.

```
[database]
  
type = sqlite3

path = grafana.db

[server]

root_url = https://grafana.mydomain/
enable_gzip = true

[security]

cookie_secure = true
cookie_samesite = true
strict_transport_security = true

[auth.google]
enabled = true
client_id = <client ID>
client_secret = <secret>
scopes = https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/userinfo.email
auth_url = https://accounts.google.com/o/oauth2/auth
token_url = https://accounts.google.com/o/oauth2/token
allowed_domains = mydomain
allow_sign_up = true

```

I had to configure Influx as a data source in Grafana using the password I
specified when I created the influx user.

With that, the query I'm using in Grafana is:

```
SELECT "generated" FROM "energyDetail" WHERE $timeFilter fill(null)
```

### slt

My fork of `slt` is [on GitHub](https://github.com/jasonmf/slt).

In an adjacent directory (e.g. `slt-image`) I build the binary and a container
image around it:

`Makefile`

```
WORK=work
CACERTS=${WORK}/ca-certificates.crt
SLT=${WORK}/slt

default: ${CACERTS} ${SLT}

${WORK}:
        mkdir -p ${WORK}

${CACERTS}: /etc/ssl/certs/ca-certificates.crt ${WORK}
        cp $< $@

${SLT}: ~/p/slt/server.go
        cd ~/p/slt
        CGO_ENABLED=0 go build -o $@ -tags netgo -ldflags="-s -w" $<
```

`Dockerfile`

```
FROM scratch

ADD work/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD work/slt /slt
ENTRYPOINT ["/slt", "/slt.conf"]
```

The `slt.conf` lives in `/var/containers/slt/slt.conf` and gets mapped in by
docker.

```
bind_addr: ":8443"
  
frontends:
  influx.lub-dub.org:
    lets_encrypt_path: /acme
    backends:
    - addr: "influx:8086"
  graf.lub-dub.org:
    lets_encrypt_path: /acme
    backends:
    - addr: "grafana:3000"
```

### Running

With everything in place, starting things up should just be:

```
docker-compose -f docker-compose.yml up -d
```

## Retrieving Data

The Go code in this repo retrieves data from the SolarEdge API and submits it
to InfluxDB. It's configured by environment variables:

 - `TRACKER_FILE`: Path to a file that the program will use to store which timestamps it has submitted values for
 - `SOLAREDGE_SITEID`: The site ID from the Admin panel
 - `SOLAREDGE_AUTH_TOKEN`: The auth token created in the Admin panel
 - `INFLUX_USER`: The user created in Influx, `solar` in the example above
 - `INFLUX_PASS`: The password created for this user
 - `INFLUX_URL`: The URL to your Influx service

 The SolarEdge API serves energy data relative to a time range. The times don't
 include a timezone and US Pacific worked for me. I don't know if the timezone
 they expect is US Pacific or the location of the solar installation.

 The API captures data in 15-minute intervals aligned to the hour. The program
 starts up then goes to sleep until one minute after the next 15-minute
 interval. The API will return values for the interval in progress so the
 program doesn't submit values that are less than 15 minutes old.

 Included is a `Makefile` and a `Dockerfile` that builds a container for the
 program. Unlike the rest of the tools, this runs on my Raspberry Pi
 Kubernetes cluster so it's built for ARM.