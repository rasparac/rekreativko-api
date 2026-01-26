FROM postgres:17.2

COPY ./fixtures/*.sql /docker-entrypoint-initdb.d/

EXPOSE 5432