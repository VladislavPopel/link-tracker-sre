# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scrapper ./cmd/scrapper \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/prober ./cmd/prober


FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scrapper /scrapper

COPY --from=build /out/prober /prober

EXPOSE 8081
ENTRYPOINT ["/scrapper"]