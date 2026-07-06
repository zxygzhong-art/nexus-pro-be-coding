ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:latest

FROM ${KEYCLOAK_IMAGE}

# 保留 upstream entrypoint；docker-compose.yml 會提供本機 start-dev command，
# 讓此 image 可在需要時用固定的 KEYCLOAK_IMAGE 重建。
