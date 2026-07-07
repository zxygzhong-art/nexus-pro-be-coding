ARG MINIO_IMAGE
FROM ${MINIO_IMAGE}

EXPOSE 9000 9001
VOLUME ["/data"]

CMD ["server", "/data", "--console-address=:9001"]
