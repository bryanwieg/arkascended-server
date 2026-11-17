FROM golang:1.25 AS build

WORKDIR /src
COPY app/ark-web/ app/ark-web/
COPY app/pkg/ark/ app/pkg/ark/
RUN go work init && go work use ./app/ark-web ./app/pkg/ark
RUN go build -o /ark-web ./app/ark-web/

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /ark-web /usr/local/bin/ark-web
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ark-web"]
