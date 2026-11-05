#[cfg(not(feature = "no-entrypoint"))]
use solana_security_txt::security_txt;

#[cfg(not(feature = "no-entrypoint"))]
security_txt! {
    name: "CCIP Lock/Release Token Pool",
    project_url: "https://chain.link",
    contacts: "link:https://hackerone.com/chainlink,link:https://immunefi.com/bounty/chainlink/",
    policy: "https://github.com/smartcontractkit/chainlink-ccip-solana/security/policy",
    source_code: "https://github.com/smartcontractkit/chainlink-ccip-solana",
    preferred_languages: "en"
}
