ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:latest

FROM ${KEYCLOAK_IMAGE}

# Keep the upstream entrypoint. docker-compose.yml supplies the local start-dev
# command so this image can be rebuilt with a pinned KEYCLOAK_IMAGE when needed.
