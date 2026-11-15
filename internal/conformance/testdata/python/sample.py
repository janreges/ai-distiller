import os

# CONF_LINE_COMMENT

class ConfWidget:
    """CONF_DOC_COMMENT"""

    conf_field = 1

    def conf_public(self):
        a = "CONF_BODY_A"
        return a

    def _conf_private(self):
        """CONF_PRIVATE_DOC"""
        return os.sep

    def conf_other(self):
        b = "CONF_BODY_B"
        return b
