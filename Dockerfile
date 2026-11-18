FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/what-track .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/what-track /what-track
WORKDIR /data
ENTRYPOINT ["/what-track"]
CMD ["-config", "/data/config.yaml"]
