from flask import Flask
from .config import Config

def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)

    # Registro de rutas basicas
    @app.route("/api/v1/health")
    def health_check():
        return {"status": "ok", "service": "controller"}, 200

    return app
