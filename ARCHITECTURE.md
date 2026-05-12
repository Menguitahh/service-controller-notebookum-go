# Arquitectura del Service-Controller-NotebookUm

**Versión**: 1.0  
**Fecha**: 2026-04-28  
**Enfoque**: Patrón Strangler, Validación en Borde, RFC 9457

---

## 1. Visión de Alto Nivel

```
┌─────────────────────────────────────────────────────────────────┐
│                        Cliente HTTP                              │
│                  (Web, Mobile, Terceros)                         │
└────────────────────────────┬──────────────────────────────────────┘
                             │
                    POST /api/v1/documento/upload
                    GET /api/v1/summaries/document/{id}
                    POST /api/v1/users
                             │
                             ▼
    ┌────────────────────────────────────────────────────────┐
    │    SERVICE-CONTROLLER-NOTEBOOKUM (este servicio)      │
    │              Puerto 5000 (Granian ASGI)               │
    ├────────────────────────────────────────────────────────┤
    │ • Valida entrada en borde (PDF, 25MB, JSON)           │
    │ • Extrae identidad del usuario (middleware)           │
    │ • Genera/propaga correlation ID (tracing distribuido) │
    │ • Enruta a destino (Strangler pattern)                │
    │ • Devuelve respuesta en RFC 9457                      │
    └────────────────────────────────────────────────────────┘
             │                │                │
      ¿Destino?    ¿Destino?    ¿Destino?
             │                │                │
   ┌─────────▼────┐   ┌──────▼────┐   ┌──────▼─────────┐
   │   Monolito   │   │Service-   │   │Service-        │
   │  Existente   │   │User       │   │Persistence     │
   │  (Fallback)  │   │           │   │                │
   └──────────────┘   └───────────┘   └────────────────┘
                            │                 │
                            └─────────┬───────┘
                                      │
                      ┌───────────────▼────────────────┐
                      │  Service-Extractor + Service-AI│
                      │  (Procesamiento asíncrono)     │
                      └────────────────────────────────┘
```

---

## 2. Flujo de Petición Detallado

```
REQUEST INGRESA
    │
    ▼
┌─────────────────────────────────┐
│ MIDDLEWARE: Correlation ID       │
│ • GET header X-Correlation-ID    │
│ • Si no existe, generar UUID     │
│ • Almacenar en g.correlation_id  │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ MIDDLEWARE: Autenticación        │
│ • GET header X-User-ID           │
│ • Si no existe, 401 Unauthorized │
│ • Almacenar en g.user_id         │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ ROUTE HANDLER (específico)       │
│ Ej: POST /api/v1/documento/...   │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ VALIDACIÓN EN BORDE              │
│ • Content-Type PDF ✓             │
│ • Tamaño ≤ 25MB ✓                │
│ • JSON válido ✓                  │
│ Si FALLA → 400 RFC 9457          │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ ORQUESTACIÓN MÍNIMA (si aplica)  │
│ • Verificar usuario existe       │
│ • Si FALLA → 404 o 401           │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ STRANGLER ROUTER                 │
│ • Leer config/strangler.yaml     │
│ • Match: POST /api/v1/... → ?    │
│ • Decidir: monolito vs MS        │
└─────────────────────────────────┘
    │
    ├─→ "monolith"
    │   ▼
    │   ┌──────────────────┐
    │   │ MonolithClient   │
    │   │ .request()       │
    │   └──────────────────┘
    │
    └─→ "service-user"
        ▼
        ┌──────────────────┐
        │ UserServiceClient│
        │ .create_user()   │
        └──────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ HTTP CLIENT                      │
│ • Construir URL                  │
│ • Propagar headers:              │
│   - X-User-ID                    │
│   - X-Correlation-ID             │
│   - Content-Type: application/json
│ • Timeout: 5s (configurable)     │
│ • Try/except: timeout → 503      │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ SERVICIO REMOTO                  │
│ Procesa y devuelve respuesta     │
│ (status_code, JSON body)         │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ ERROR HANDLER                    │
│ • Si 5xx → Envolver en RFC 9457  │
│ • Si 4xx → Pasar o Envolver      │
│ • Si timeout → 503 RFC 9457      │
└─────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────┐
│ RESPONSE FORMATTER               │
│ • Status code correcto           │
│ • Header X-Correlation-ID        │
│ • Body en JSON RFC 9457 si error │
└─────────────────────────────────┘
    │
    ▼
RESPONSE DEVUELVE AL CLIENTE
(Con status_code, headers, body JSON)
```

---

## 3. Estructura de Directorios

```
service-controller-notebookum/
├── app/                          # Código de aplicación
│   ├── __init__.py               # Factory: create_app()
│   ├── config.py                 # Configuración (env vars, defaults)
│   │
│   ├── routes/                   # HTTP Routes (Blueprints)
│   │   ├── __init__.py
│   │   ├── health.py             # GET /health
│   │   ├── users.py              # POST/GET /api/v1/users
│   │   ├── documents.py          # POST /api/v1/documento/upload
│   │   └── summaries.py          # GET /api/v1/summaries/document/{id}
│   │
│   ├── middleware/               # Middleware (request interceptors)
│   │   ├── __init__.py
│   │   ├── correlation.py        # X-Correlation-ID generation/propagation
│   │   ├── auth.py               # X-User-ID extraction, @require_auth
│   │   └── error_handler.py      # RFC 9457 formatting, global error handlers
│   │
│   ├── validators/               # Input validation (edge)
│   │   ├── __init__.py
│   │   ├── file_validator.py     # PDF type, file size validation
│   │   └── json_validator.py     # JSON structure validation
│   │
│   ├── services/                 # Business logic
│   │   ├── __init__.py
│   │   ├── strangler_config.py   # Load/parse strangler.yaml
│   │   └── strangler_router.py   # Route decision, client selection
│   │
│   ├── clients/                  # HTTP clients to remote services
│   │   ├── __init__.py
│   │   ├── base_client.py        # BaseClient (timeout, error handling)
│   │   ├── user_service_client.py
│   │   ├── persistence_client.py
│   │   ├── extractor_client.py
│   │   └── monolith_client.py
│   │
│   └── utils/                    # Utilities
│       ├── __init__.py
│       ├── logger.py             # JSON logging setup
│       └── metrics.py            # Prometheus metrics
│
├── tests/                        # Test suite
│   ├── conftest.py              # pytest fixtures
│   ├── test_app_init.py
│   ├── validators/
│   │   └── test_file_validator.py
│   ├── middleware/
│   │   ├── test_auth.py
│   │   ├── test_correlation.py
│   │   └── test_error_handler.py
│   ├── services/
│   │   ├── test_strangler_config.py
│   │   └── test_strangler_router.py
│   ├── clients/
│   │   ├── test_base_client.py
│   │   └── test_user_service_client.py
│   ├── routes/
│   │   ├── test_users.py
│   │   ├── test_documents.py
│   │   └── test_summaries.py
│   └── integration/
│       ├── test_e2e_create_user.py
│       └── test_with_docker.py
│
├── config/                      # Configuration files
│   └── strangler.yaml          # Routing rules (monolith vs MS)
│
├── .env.example                # Environment variables template
├── Dockerfile                  # Container image
├── docker-compose.yml          # Local development (Traefik, services)
├── pyproject.toml             # Dependencies (Flask, requests, pytest)
├── main.py                     # Entry point (if Granian)
│
├── spec.md                     # Especificación técnica (SDD)
├── plan.md                     # Plan de implementación
├── tasks.md                    # Catálogo de tareas (TDD)
├── SUMMARY.md                  # Este documento
└── ARCHITECTURE.md             # Documentación arquitectónica
```

---

## 4. Componentes Principales y Responsabilidades

### 4.1 HTTP Handler (routes/)

**Responsabilidad**: Exponer endpoints HTTP `/api/v1/*`

**Patrón**:
```python
@bp.route('/api/v1/endpoint', methods=['POST'])
@require_auth  # Middleware decorator
def endpoint_handler():
    # 1. Validar entrada
    if not validate(request.json):
        return error_400(...)
    
    # 2. Orquestar si es necesario
    status, data = verify_something()
    if status != 200:
        return error_response(status, ...)
    
    # 3. Enrutar via Strangler
    status, response = router.route_request(...)
    
    # 4. Devolver
    return jsonify(response), status
```

**No debe**:
- Validar tokens complejos (delegado a `service-user`)
- Persistir datos (delegado a `service-persistence`)
- Procesar PDFs (delegado a `service-extractor`)

---

### 4.2 Middleware

#### Correlation ID (correlation.py)
- **Entrada**: Request sin/con `X-Correlation-ID`
- **Salida**: `g.correlation_id` configurado, header de respuesta establecido
- **Propósito**: Tracing distribuido, correlacionar logs entre servicios

#### Autenticación (auth.py)
- **Entrada**: Request con/sin `X-User-ID` o `Authorization`
- **Salida**: `g.user_id` configurado o 401
- **Propósito**: Extraer contexto de usuario, validar acceso

#### Error Handler (error_handler.py)
- **Entrada**: Excepción no manejada (400, 403, 404, 500, etc.)
- **Salida**: Response RFC 9457 (type, title, status, detail, instance)
- **Propósito**: Uniformizar formato de error, ocultar detalles internos

---

### 4.3 Validadores (validators/)

**Imperativa** (no declarativa):

```python
def validate_pdf_content_type(content_type):
    if not content_type:
        return False
    base_type = content_type.split(';')[0].strip().lower()
    return base_type == 'application/pdf'
```

**Beneficios**:
- Testeable (unit tests fáciles)
- Sin dependencias pesadas (pydantic, jsonschema)
- KISS (simple de entender)

---

### 4.4 Strangler Router (strangler_router.py)

**Flujo**:
1. Carga config YAML (mapeo de rutas)
2. Match request (method + path) con patrón
3. Obtener `destination` (monolith vs service-X)
4. Obtener cliente (BaseClient subclass)
5. Ejecutar request remoto
6. Devolver (status, response)

**Config YAML**:
```yaml
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "service-user"
    enabled: true
  - route_pattern: "GET /api/v1/users/:id"
    destination: "service-user"
    enabled: true
  - route_pattern: "POST /api/v1/documento/upload"
    destination: "service-extractor"
    enabled: true
```

**Cambio dinámico**:
- Cambiar `destination: "service-user"` → `destination: "monolith"`
- Nueva petición usará nuevo destino
- Sin necesidad de reiniciar (si config es hot-reload; MVP: redeploy)

---

### 4.5 HTTP Clients (clients/)

**Patrón**:
```
BaseClient
  ├── timeout handling
  ├── error mapping (timeout→503, connection→503, 5xx→502)
  └── header propagation
      │
      ├── UserServiceClient
      ├── PersistenceServiceClient
      ├── ExtractorServiceClient
      └── MonolithClient
```

**Responsabilidades**:
- Construir URL (base_url + path)
- Propagar headers (X-User-ID, X-Correlation-ID)
- Manejar timeout (5s por defecto)
- Mapear errores a status HTTP coherentes

---

## 5. Flujos de Caso de Uso Principal

### Caso 1: Crear Usuario

```
POST /api/v1/users
  ├─ Validación JSON (name, email)
  ├─ Strangler → destination
  │  ├─ "service-user" → UserServiceClient.create_user()
  │  └─ "monolith" → MonolithClient.request()
  ├─ Response 201 + {id, name, email}
  └─ Logs: correlation_id, user_id, status
```

### Caso 2: Cargar Documento PDF

```
POST /api/v1/documento/upload (file, user_id)
  ├─ Validación Content-Type (application/pdf)
  ├─ Validación tamaño (≤25MB)
  ├─ Verificar usuario existe
  │  └─ UserServiceClient.get_user() → 200 ✓ o 404 ✗
  ├─ Strangler → destination
  │  └─ "service-extractor" → ExtractorServiceClient.upload_document()
  ├─ Response 202 + {document_id, status: 'accepted'}
  └─ Logs: document_id, user_id, file_size
```

### Caso 3: Consultar Resumen

```
GET /api/v1/summaries/document/{doc_id}
  ├─ Validar identidad (user_id via middleware)
  ├─ Obtener documento
  │  └─ PersistenceServiceClient.get_document() → {owner_id, summary}
  ├─ Validar propietario (owner_id == user_id)
  │  ├─ NO → 403 Forbidden
  │  └─ SÍ → Continuar
  ├─ Response
  │  ├─ Si summary → 200 + {summary}
  │  └─ Si no → 202 "aún procesando"
  └─ Logs: doc_id, user_id, status
```

---

## 6. Patrones de Seguridad

### Validación de Entrada (Defense in Depth)

```
Controller (borde)              Servicios (también validan)
  │                              │
  ├─ Content-Type PDF            │
  ├─ Tamaño ≤25MB                ├─ Revalidar tipo PDF
  ├─ JSON schema básico           ├─ Validar contenido
  ├─ User ID presente             └─ Persistir con integridad
  └─ Formato email
```

### Control de Acceso

```
Recurso propiedad de User A
  │
  ├─ User A accede → ✓ Permitido
  ├─ User B accede → ✗ 403 Forbidden (sin revelar detalles)
  └─ Anonymous accede → ✗ 401 Unauthorized
```

### Propagación de Contexto

```
Controller                 Service-X              Service-Y
  │                           │                     │
  ├─ X-User-ID: user123      ├─ Lee X-User-ID    ├─ Lee X-User-ID
  ├─ X-Correlation-ID: uuid  ├─ Log con uuid     └─ Log con uuid
  └─────────────────────────→ ├─ Delega a Y      
                              └─────────────────→
```

---

## 7. Modelo de Datos (Conceptual)

### Transporte entre servicios

```
CreateUserRequest:
  {
    "name": "John Doe",
    "email": "john@example.com"
  }

CreateUserResponse:
  {
    "id": "uuid-123",
    "name": "John Doe",
    "email": "john@example.com",
    "created_at": "2026-04-28T..."
  }

UploadDocumentRequest:
  {
    "document_id": "uuid-456",
    "filename": "document.pdf",
    "file_size": 1024000,
    "user_id": "uuid-123"
  }

UploadDocumentResponse (202):
  {
    "document_id": "uuid-456",
    "status": "accepted",
    "message": "Processing..."
  }

GetSummaryResponse:
  {
    "document_id": "uuid-456",
    "summary": "Resumen del documento...",
    "status": "ready"
  }
```

---

## 8. Configuración Strangler (Configuración de Enrutamiento)

### Ejemplo: Migración gradual

**Día 1** (Todos en monolito):
```yaml
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "monolith"
    enabled: true
  - route_pattern: "POST /api/v1/documento/upload"
    destination: "monolith"
    enabled: true
```

**Día 5** (Users migrado):
```yaml
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "service-user"  # ← Cambiado
    enabled: true
  - route_pattern: "GET /api/v1/users/:id"
    destination: "service-user"  # ← Cambiado
    enabled: true
  - route_pattern: "POST /api/v1/documento/upload"
    destination: "monolith"      # ← Aún aquí
    enabled: true
```

**Día 10** (Todo en microservicios):
```yaml
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "service-user"
    enabled: true
  - route_pattern: "GET /api/v1/users/:id"
    destination: "service-user"
    enabled: true
  - route_pattern: "POST /api/v1/documento/upload"
    destination: "service-extractor"  # ← Migrado
    enabled: true
  - route_pattern: "GET /api/v1/summaries/document/:id"
    destination: "service-persistence"  # ← Migrado
    enabled: true
```

**Rollback rápido** (Si falla service-user):
```yaml
rules:
  - route_pattern: "POST /api/v1/users"
    destination: "monolith"  # ← Revertir
    enabled: true
```

---

## 9. Propiedades No Funcionales

### Performance
- Validación en borde: < 100ms (Content-Type, tamaño)
- Enrutamiento: < 50ms (config matching)
- Total request: < 1s para validación + enrutamiento

### Confiabilidad
- Timeout a microservicios: 5s (configurable)
- Error handling: timeout → 503, connection error → 503, 5xx → 502
- Reintentos: 1 intento simple (no exponential backoff en MVP)

### Observabilidad
- Logs JSON: {timestamp, level, service, message, user_id, correlation_id, status_code}
- Métricas: requests_total, request_duration (histograma)
- Health check: GET /health < 100ms

### Seguridad
- HTTPS obligatorio (en Traefik/nginx)
- Headers de seguridad (X-Content-Type-Options, X-Frame-Options)
- No loguear passwords, tokens
- Validación de entrada 100%

---

## 10. Decisiones de Diseño Clave

| Decisión | Por qué | Alternativa |
|---|---|---|
| **Flask** | Ligero, en pyproject.toml | FastAPI (overhead innecesario) |
| **YAML Strangler** | Simple, auditable | Etcd/Consul (YAGNI) |
| **Sin Circuit Breaker** | Try/except + 503 es suficiente | pybreaker (complejidad) |
| **Validación Imperativa** | Simple, sin dependencias | Pydantic (overhead) |
| **BaseClient + subclases** | Reutilización, modularidad | Cliente monolítico (inflexible) |
| **Middleware globales** | Limpio, DRY (no duplication) | Auth en cada handler (duplication) |
| **Logging JSON** | Observable, estructurado | print() (no parseble) |
| **TDD obligatorio** | Calidad, confiabilidad | Código primero (deuda técnica) |

---

**Fin de ARCHITECTURE.md**
