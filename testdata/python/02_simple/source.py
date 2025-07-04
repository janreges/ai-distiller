"""
Represents a user account in the system.
"""
from datetime import datetime

class User:
    """A simple representation of a user with basic attributes."""

    def __init__(self, user_id: int, username: str, email: str):
        if not username or not email:
            raise ValueError("Username and email cannot be empty.")
        
        self.user_id = user_id
        self.username = username
        self.email = email
        self.created_at: datetime = datetime.utcnow()
        self._is_active = True

    def deactivate(self) -> None:
        """Marks the user as inactive."""
        self._is_active = False

    def get_status(self) -> str:
        """Returns the current status of the user."""
        return "Active" if self._is_active else "Inactive"

    def __repr__(self) -> str:
        return f"<User id={self.user_id} username='{self.username}'>"