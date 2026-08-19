FROM <docker_image>

RUN apk add --no-cache make

WORKDIR /app
COPY go.mod .
RUN go mod download

COPY . .
RUN make build && \
    mkdir -p /go/bin && \
    mv -v bin/ctop /go/bin/

FROM scratch
ENV TERM=linux
COPY --from=0 /go/bin/ctop /ctop
ENTRYPOINT ["/ctop"]
