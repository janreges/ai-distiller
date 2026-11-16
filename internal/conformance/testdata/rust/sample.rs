#![allow(non_upper_case_globals)]

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
    }

    /// CONF_PRIVATE_DOC
    fn conf_private(&self) -> char {
        MAIN_SEPARATOR
    }

    pub fn conf_other(&self) -> &'static str {
        let b = "CONF_BODY_B";
        b
    }

    pub fn conf_quick(&self) -> i32 { 1 }
}

pub const conf_max: u32 = 5;

pub enum ConfColor { ConfRed, ConfGreen }

pub trait ConfShape {
    fn conf_area(&self) -> i32;
}

impl ConfShape for ConfWidget {
    fn conf_area(&self) -> i32 {
        let c = "CONF_BODY_C";
        1
    }
}

struct ConfHidden<'a> {
    conf_hidden_field: &'a str,
}

/// CONF_HIDDEN_IMPL_DOC
impl<'a> ConfHidden<'a> {
    pub fn conf_hidden_method(&self) {}
}

impl ConfShape for ConfHidden<'_> {
    fn conf_area(&self) -> i32 { 2 }
}

pub(crate) struct ConfCrate;

impl ConfCrate {
    pub fn conf_crate_method(&self) {}
}

pub use crate::conf_errors::ConfImported;

impl ConfImported {
    pub fn conf_imported_method(&self) {}
}

mod conf_engine {
    pub struct ConfEngine;
}

pub use conf_engine::ConfEngine;

impl ConfEngine {
    pub fn conf_engine_start(&self) {}
}

#[cfg(test)]
mod conf_tests {
    struct ConfImported;
}
