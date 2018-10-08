FROM alpine:3.8
RUN apk add --no-cache --virtual .build-deps ca-certificates

COPY build/github-webhook-dco-labeler /github-webhook-dco-labeler

CMD ["/github-webhook-dco-labeler"]