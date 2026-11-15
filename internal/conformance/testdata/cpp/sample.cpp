#include <string>

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
class ConfWidget {
public:
    int confField = 1;

    std::string confPublic() {
        std::string a = "CONF_BODY_A";
        return a;
    }

private:
    std::string confHiddenOut();

    /** CONF_PRIVATE_DOC */
    std::string confPrivate() {
        return std::string(1, '/');
    }

public:
    std::string confOut();

    std::string confOther() {
        std::string b = "CONF_BODY_B";
        return b;
    }
};

std::string ConfWidget::confOut() { return "CONF_BODY_C"; }

std::string ConfWidget::confHiddenOut() { return "y"; }

namespace confns {
class ConfInner {
public:
    int confInnerOpen();

private:
    int confNsSecret();
};

int ConfInner::confInnerOpen() { return 1; }

int ConfInner::confNsSecret() { return 2; }
}
