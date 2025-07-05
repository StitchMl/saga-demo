# frontend/Dockerfile

# Stage 1: build static assets (opzionale, qui non abbiamo tool di build)
# usiamo direttamente nginx per servire i file statici
FROM nginx:alpine

# Rimuoviamo la configurazione di default
RUN rm /etc/nginx/conf.d/default.conf

# Copiamo la nostra configurazione
COPY nginx.conf /etc/nginx/conf.d/

# Copiamo i file statici nella directory di nginx
COPY . /usr/share/nginx/html

# Espone la porta 80
EXPOSE 80

# Avvia nginx in foreground
CMD ["nginx", "-g", "daemon off;"]