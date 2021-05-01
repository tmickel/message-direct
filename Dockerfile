FROM debian:buster-slim

COPY out/* .
CMD ["./app"]
