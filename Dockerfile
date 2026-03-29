FROM golang:1.26.1 AS build
ENV PATH=/usr/local/go/bin:$PATH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/regula ./cmd/regula

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/regula /regula
COPY --from=build /app/seed /seed
EXPOSE 8085
ENTRYPOINT ["/regula"]
