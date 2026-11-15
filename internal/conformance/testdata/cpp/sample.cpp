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
    /** CONF_PRIVATE_DOC */
    std::string confPrivate() {
        return std::string(1, '/');
    }

public:
    std::string confOther() {
        std::string b = "CONF_BODY_B";
        return b;
    }
};
