# Stage 1: build static assets (optional, we have no build tool here)
# we use nginx directly to serve the static files
FROM nginx:alpine

# Install envsubst, which is part of the gettext-base package.
# This tool allows you to replace environment variables in the Nginx configuration file.
# We remove the default configuration of Nginx.
RUN apk add --no-cache gettext-base; rm /etc/nginx/conf.d/default.conf

# We copy our Nginx configuration file as a template.
# Environment variables will be replaced by envsubst at runtime.
COPY nginx.conf /etc/nginx/conf.d/nginx.conf.template

# We copy the application's static files (HTML, CSS, JS) into the Nginx directory.
COPY . /usr/share/nginx/html

# It exposes port 80, the default port for HTTP traffic.
EXPOSE 80

# Start Nginx in the foreground.
# Before starting Nginx, envsubst reads the configuration template file
# and replaces the environment variables (e.g. $ORCHESTRATOR_HOST) with their actual values.
# The result is redirected to /etc/nginx/conf.d/default.conf, which Nginx will load.
CMD ["/bin/sh", "-c", "envsubst < /etc/nginx/conf.d/nginx.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]