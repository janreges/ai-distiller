import os

# CONF_LINE_COMMENT

class ConfWidget:
    """CONF_DOC_COMMENT"""

    conf_field = 1

    def conf_public(self):
        """CONF_FN_DOC"""
        a = "CONF_BODY_A"
        return a

    def _conf_private(self):
        """CONF_PRIVATE_DOC"""
        return os.sep

    def conf_other(self):
        b = "CONF_BODY_B"
        return b


class ConfRecord:
    conf_id: int
    conf_name: str = "x"
