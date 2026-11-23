FROM nginxinc/nginx-unprivileged:stable-alpine
USER root
# L'image de base épingle nginx (=version dans /etc/apk/world) et embarque des modules
# (njs, geoip, xslt, image-filter) qui exigent nginx~1.30.6 sans version plus récente dans le
# dépôt. Aucun n'est chargé (pas de load_module) : on les retire et on lève l'épinglage,
# sinon apk upgrade ne peut pas installer les correctifs de sécurité de nginx.
RUN apk del --no-cache nginx-module-njs nginx-module-geoip nginx-module-xslt nginx-module-image-filter \
 && sed -i 's/=.*//' /etc/apk/world \
 && apk upgrade --no-cache
USER 101
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY static/ /usr/share/nginx/html/
EXPOSE 8080
