FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
# next.config.ts bakes the rewrite destination into the build, so the API
# address must be known here, not only at runtime.
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOSTNAME=0.0.0.0
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
RUN adduser -D -u 10001 app
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
USER app
EXPOSE 3000
CMD ["node", "server.js"]
