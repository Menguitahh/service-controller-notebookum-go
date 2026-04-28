import requests
from flask import Flask, jsonify
from .config import Config

def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)

    # Registro de rutas basicas
    @app.route("/api/v1/health")
    def health_check():
        return {"status": "ok", "service": "controller"}, 200

    @app.route("/api/v1/health/all")
    def health_check_all():
        services = {
            "extractor": app.config.get("EXTRACTOR_URL") + "/health",
            "ai": app.config.get("AI_URL") + "/health",
            "persistence": app.config.get("PERSISTENCE_URL") + "/health",
            "user": app.config.get("USER_SERVICE_URL") + "/health"
        }
        results = {}
        for name, url in services.items():
            try:
                # El controlador hace una peticion HTTP a cada microservicio
                response = requests.get(url, timeout=3)
                results[name] = {"status": "online", "data": response.json()}
            except Exception as e:
                results[name] = {"status": "offline", "error": str(e)}
        
        return jsonify({
            "controller": "online",
            "microservices": results
        }), 200

    return app
