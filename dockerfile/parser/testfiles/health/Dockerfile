FROM debian
ADD check.sh main.sh /app/
CMD /app/main.sh
HEALTHCHECK
HEALTHCHECK --interval=5s --timeout=3s --retries=3 \
  CMD /app/check.sh --quiet
HEALTHCHECK CMD
HEALTHCHECK   CMD   a b
HEALTHCHECK --timeout=3s CMD ["foo"]
HEALTHCHECK CONNECT TCP 7000
HEALTHCHECK --start-period=0s --start-interval=0s --interval=5s --timeout=0s --retries=0 CMD ["foo"]
