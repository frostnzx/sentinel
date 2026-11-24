FROM golang:1.24-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/simulator ./cmd/simulator

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/simulator /usr/local/bin/simulator
COPY backend/migrations ./migrations
COPY contracts ./contracts
CMD ["api"]
