import json

from models import User
from services.mailer import send_welcome


def main() -> None:
    """Create a user and welcome them."""
    user = User("ada")
    print(json.dumps({"sent": send_welcome(user)}))
