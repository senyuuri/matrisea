FROM node:17-alpine

WORKDIR /app
COPY . .
RUN yarn install
CMD ["yarn", "docker-start"]