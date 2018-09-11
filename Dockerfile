FROM debian:9.5-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY build/github-webhook-dco-labeler /github-webhook-dco-labeler

CMD ["/github-webhook-dco-labeler"]