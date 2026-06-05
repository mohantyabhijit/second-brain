FROM golang:1.23-bookworm AS build

WORKDIR /src/backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/second-brain-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app/backend

COPY --from=build /out/second-brain-api /usr/local/bin/second-brain-api

EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/second-brain-api"]
