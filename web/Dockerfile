# syntax=docker/dockerfile:1.7
#
# The web UI served by nginx in front of the API.
#
# Two stages, pinned to the versions this repository pins elsewhere:
#
#   build  node 24.22.1  (mise.toml)
#   final  nginx:alpine
#
# The build context is this folder, not the repository root, so the
# `web/.dockerignore` next to this file keeps host `node_modules` and a stale
# `dist` out of the image.
#
# The UI calls /v1/* relatively, so nginx proxies it to the API. Both tiers are
# published on their own ports in docker compose (web 5173, API 4000).

ARG NODE_VERSION=24.22.1
ARG NGINX_VERSION=alpine

FROM node:${NODE_VERSION}-bookworm-slim AS build
WORKDIR /src/web
# The manifests first: the dependency install then survives a source change,
# and only a lockfile change invalidates that layer.
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:${NGINX_VERSION}
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/web/dist /usr/share/nginx/html

EXPOSE 80
