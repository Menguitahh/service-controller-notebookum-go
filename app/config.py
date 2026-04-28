import os
from dotenv import load_dotenv

load_dotenv()

class Config:
    SECRET_KEY = os.environ.get("SECRET_KEY", "dev-secret-key")
    EXTRACTOR_URL = os.environ.get("EXTRACTOR_URL", "http://extractor:5001")
    AI_URL = os.environ.get("AI_URL", "http://ai:5002")
    PERSISTENCE_URL = os.environ.get("PERSISTENCE_URL", "http://persistence:5003")
    USER_SERVICE_URL = os.environ.get("USER_SERVICE_URL", "http://user-service:5004")
