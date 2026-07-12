# syntax=docker/dockerfile:1.7
FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags='-s -w' -o /out/secureledger ./cmd/secureledger && \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags='-s -w' -o /out/secureledger-healthcheck ./cmd/secureledger-healthcheck && \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags='-s -w' -o /out/secureledger-reconcile ./cmd/secureledger-reconcile

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/secureledger /secureledger
COPY --from=build /out/secureledger-healthcheck /secureledger-healthcheck
COPY --from=build /out/secureledger-reconcile /secureledger-reconcile
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/secureledger"]
