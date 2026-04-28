FROM python:3.12-slim

# Establecer directorio de trabajo
WORKDIR /app

# Instalar dependencias del sistema requeridas
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Instalar uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /bin/uv

# Copiar archivos de dependencias
COPY pyproject.toml uv.lock* ./

# Instalar dependencias (usando uv)
RUN uv sync

# Copiar el resto del codigo fuente
COPY . .

# Exponer el puerto
EXPOSE 5000

# Comando para iniciar con Granian
CMD ["uv", "run", "granian", "--interface", "wsgi", "main:app", "--host", "0.0.0.0", "--port", "5000", "--workers", "2"]
