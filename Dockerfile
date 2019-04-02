FROM golang:1.12 as build
WORKDIR /opt/vdc
COPY . .
RUN CGO_ENABLED=0 go build -a --installsuffix cgo --ldflags="-w -s -X main.Build=$(git rev-parse --short HEAD)" -o mock-vdc

FROM alpine
COPY --from=build /opt/vdc/mock-vdc /opt/vdc/mock-vdc
EXPOSE 8080
ENTRYPOINT [ "/opt/vdc/mock-vdc" ,"--port","8080"]