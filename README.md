# Service Controller NotebookUm

Este microservicio actúa como **Controlador de Llamadas** (API Gateway / Orquestador) para el sistema NotebookUm.

## Responsabilidades
- Recibe las peticiones HTTP externas.
- Delega el trabajo a los otros microservicios (Extractor, IA, Persistencia, Usuario).
- Agrupa las respuestas y las devuelve al cliente.

## Ejecución con Docker
```bash
docker-compose up -d --build
```
El servicio estará disponible en el puerto `5000` (o a través de Traefik en `controller.universidad.localhost`).
