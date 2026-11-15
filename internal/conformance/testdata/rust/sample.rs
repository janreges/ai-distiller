use std::path::MAIN_SEPARATOR;

// CONF_LINE_COMMENT

/// CONF_DOC_COMMENT
pub struct ConfWidget {
    pub conf_field: i32,
}

impl ConfWidget {
    pub fn conf_public(&self) -> &'static str {
        let a = "CONF_BODY_A";
        a

    /// CONF_PRIVATE_DOC
    fn conf_private(&self) -> char {
        MAIN_SEPARATOR
    }
    }

    pub fn conf_other(&self) -> &'static str {
        let b = "CONF_BODY_B";
        b
    }
}
