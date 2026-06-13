FROM node:18

WORKDIR /app

COPY package*.json /app/
RUN npm install

COPY . /app

EXPOSE 5173

CMD ["sh", "-c", "npm run dev"]
