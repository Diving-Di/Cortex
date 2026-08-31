ARG NODE_BUILDER_IMAGE=node:20-alpine
ARG NGINX_RUNTIME_IMAGE=nginx:1.27-alpine
FROM ${NODE_BUILDER_IMAGE} AS build

WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM ${NGINX_RUNTIME_IMAGE}

LABEL org.opencontainers.image.title="Cortex Frontend" \
      org.opencontainers.image.description="Cortex personal knowledge interface"

RUN apk upgrade --no-cache

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

USER nginx

EXPOSE 8080
