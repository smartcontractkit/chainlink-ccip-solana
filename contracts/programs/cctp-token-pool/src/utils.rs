use std::fmt::Display;

#[derive(PartialEq, Eq)]
pub struct DisplaySlice<'a>(pub &'a [u8]);
impl Display for DisplaySlice<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        for byte in self.0 {
            write!(f, "{:02x}", byte)?;
        }
        Ok(())
    }
}
