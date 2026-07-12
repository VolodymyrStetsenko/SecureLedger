# syntax=docker/dockerfile:1.7
FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags='-s -w' -o /out/secureledger ./cmd/secureledger

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/secureledger /secureledger
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/secureledger"]
