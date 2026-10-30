FROM golang:1.27 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go generate ./src
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/todomvc ./cmd/todomvc

FROM gcr.io/distroless/static-debian12

COPY --from=build /out/todomvc /todomvc
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/todomvc"]
