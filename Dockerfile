# A static Go binary, no Chrome, nothing else in the image: this is the
# service that holds Instagram access, so it is kept as small as it can be.
#   docker build --platform linux/amd64 -t instagram-publisher .
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /instagram-publisher ./cmd/instagram-publisher

# distroless/static has CA certificates (Instagram and the secret store are
# https) and nothing else; nonroot is its default user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /instagram-publisher /instagram-publisher
# Cloud Run sets PORT.
ENTRYPOINT ["/instagram-publisher"]
CMD ["serve"]
