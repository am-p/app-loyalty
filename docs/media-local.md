# Imágenes en desarrollo local

El archivo opcional `docker-compose.media.local.yml` agrega almacenamiento S3
privado con MinIO y crea el bucket antes de iniciar la API. Guarda los archivos
en el volumen `media-data`; el servicio solo se publica en `127.0.0.1:9000`.
Las imágenes Docker se construyen desde versiones fijadas del código oficial
de MinIO y su cliente, por lo que el primer inicio tarda más.

Configurar en el `.env` local (no versionar credenciales):

```dotenv
APP_ENV=development
MEDIA_PROVIDER=s3
S3_ENDPOINT=http://media:9000
S3_PUBLIC_ENDPOINT=http://localhost:9000
S3_REGION=us-east-1
S3_BUCKET=puntazo-media
S3_ACCESS_KEY_ID=puntazo-local
S3_SECRET_ACCESS_KEY=<contraseña local aleatoria de al menos 32 caracteres>
S3_SERVER_SIDE_ENCRYPTION=DISABLED
```

Iniciar siempre con ambos archivos para mantener las dependencias de la API:

```powershell
docker compose --env-file .env -f docker-compose.yml -f docker-compose.media.local.yml up -d --build
```

La API firma enlaces temporales de lectura; el bucket no permite acceso anónimo.
`localhost` corresponde al equipo que abre la app. Para probar desde otro
dispositivo hace falta una dirección alcanzable y ajustar la exposición del
servicio. Esta configuración es exclusivamente local: producción conserva
sus requisitos de HTTPS, cifrado y credenciales propios.
