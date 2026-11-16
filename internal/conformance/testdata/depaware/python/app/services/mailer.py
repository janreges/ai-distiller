from ..models import User
from .templates import render


def send_welcome(user: User) -> bool:
    body = render("welcome", user.greeting())
    return bool(body)
