FROM nginx:alpine

# Install gettext-base for envsubst
RUN apk add --no-cache gettext-base; rm /etc/nginx/conf.d/default.conf

# Copy the Nginx template configuration
COPY nginx.conf.template /etc/nginx/conf.d/default.conf.template

# Copy the static frontend files
COPY . /usr/share/nginx/html

# Use envsubst to replace ONLY the specified environment variables in the Nginx config at runtime.
# The '$$' escapes the '$' for the shell, so envsubst sees '$VAR_NAME' correctly.
CMD ["/bin/sh", "-c", "envsubst '$$ORCHESTRATOR_HOST $$ORDER_SERVICE_CHOREO_HOST $$PAYMENT_SERVICE_CHOREO_HOST $$SHIPPING_SERVICE_CHOREO_HOST' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]

EXPOSE 80