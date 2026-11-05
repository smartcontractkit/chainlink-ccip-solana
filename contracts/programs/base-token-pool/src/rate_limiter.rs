use anchor_lang::prelude::*;
use anchor_lang::solana_program::log::sol_log;

use crate::errors::CcipTokenPoolError;

// NOTE: spl-token or spl-token-2022 use u64 for amounts

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct RateLimitTokenBucket {
    pub tokens: u64, // Current number of tokens that are in the bucket. Represents how many tokens a single transaction can consume in a single transaction - different than total number of tokens within a pool
    pub last_updated: u64, // Timestamp in seconds of the last token refill, good for 100+ years.
    cfg: RateLimitConfig,
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct RateLimitConfig {
    pub enabled: bool, // Indication whether the rate limiting is enabled or not
    pub capacity: u64, // Maximum number of tokens that can be in the bucket.
    pub rate: u64,     // Number of tokens per second that the bucket is refilled.
}

pub trait Timestamper: Sized {
    fn unix_timestamp(&self) -> u64;
    fn instance() -> std::result::Result<Self, ProgramError>;
}

impl Timestamper for Clock {
    fn unix_timestamp(&self) -> u64 {
        // SAFETY: Solana clock unix_timestamp is always positive (post-1970)
        self.unix_timestamp as u64
    }

    fn instance() -> std::result::Result<Self, ProgramError> {
        Self::get()
    }
}

impl RateLimitTokenBucket {
    pub fn is_enabled(&self) -> bool {
        self.cfg.enabled
    }

    pub fn config(&self) -> &RateLimitConfig {
        &self.cfg
    }

    /// Returns a copy of this bucket refilled up to the current time, leaving the stored bucket
    /// untouched. Read-only observation instructions use this so that consumers see the tokens
    /// actually available now rather than the balance as of the last `consume`, matching EVM's
    /// `getCurrentInboundRateLimiterState`/`getCurrentOutboundRateLimiterState`.
    pub fn as_of_now<C: Timestamper>(&self) -> Result<Self> {
        let mut refilled = self.clone();
        refilled.refill::<C>()?;
        Ok(refilled)
    }

    // consume calculates the new amount of tokens in a bucket (up to the capacity)
    // and validates that the bucket has enough to fulfill the request
    pub fn consume<C: Timestamper>(&mut self, request_tokens: u64) -> Result<()> {
        if !self.cfg.enabled || request_tokens == 0 {
            return Ok(());
        }

        let capacity = self.cfg.capacity;
        require_gte!(
            capacity,
            request_tokens,
            CcipTokenPoolError::RLMaxCapacityExceeded,
        );

        self.refill::<C>()?;

        if self.tokens < request_tokens {
            let rate = self.cfg.rate;
            require_neq!(rate, 0, CcipTokenPoolError::RLRateLimitReached);
            // Wait required until the bucket is refilled enough to accept this value, round up to next higher second
            // Consume is not guaranteed to succeed after wait time passes if there is competing traffic.
            // This acts as a lower bound of wait time.

            let min_wait_sec = (request_tokens
                .checked_sub(self.tokens)
                .ok_or(CcipTokenPoolError::RLArithmeticError)?
                .saturating_add(
                    rate.checked_sub(1)
                        .ok_or(CcipTokenPoolError::RLArithmeticError)?,
                ))
            .checked_div(rate)
            .ok_or(CcipTokenPoolError::RLArithmeticError)?;

            // print human readable message before reverting with total wait time
            let msg = "min wait (s): ".to_owned() + &min_wait_sec.to_string();
            sol_log(&msg);

            return Err(CcipTokenPoolError::RLRateLimitReached.into());
        }

        self.tokens = self
            .tokens
            .checked_sub(request_tokens)
            .ok_or(CcipTokenPoolError::RLArithmeticError)?;
        emit!(TokensConsumed {
            tokens: request_tokens
        });

        Ok(())
    }

    fn refill<C: Timestamper>(&mut self) -> Result<()> {
        let current_timestamp = C::instance()?.unix_timestamp();
        let time_diff = current_timestamp.saturating_sub(self.last_updated);
        let increase = time_diff.saturating_mul(self.cfg.rate);
        let refill = self.tokens.saturating_add(increase);

        self.tokens = std::cmp::min(self.cfg.capacity, refill);
        self.last_updated = current_timestamp;
        Ok(())
    }

    // set_token_bucket_config validates a new config and resets the bucket to it
    pub fn set_token_bucket_config(&mut self, config: RateLimitConfig) -> Result<()> {
        self.set_token_bucket_config_at::<Clock>(config)
    }

    fn set_token_bucket_config_at<C: Timestamper>(
        &mut self,
        config: RateLimitConfig,
    ) -> Result<()> {
        validate_token_bucket_config(&config)?;
        // As EVM's `RateLimiter._setTokenBucketConfig`: the bucket starts full under the new config,
        // and nothing held or elapsed before the update carries over.
        *self = Self {
            tokens: config.capacity,
            last_updated: C::instance()?.unix_timestamp(),
            cfg: config.clone(),
        };

        emit!(ConfigChanged { config });

        Ok(())
    }
}

fn validate_token_bucket_config(cfg: &RateLimitConfig) -> Result<()> {
    if cfg.enabled {
        require_gte!(
            cfg.capacity,
            cfg.rate, // allow setting rate and capacity to 0
            CcipTokenPoolError::RLInvalidRateLimitRate
        );
    } else {
        require_eq!(cfg.rate, 0, CcipTokenPoolError::RLDisabledNonZeroRateLimit);
        require_eq!(
            cfg.capacity,
            0,
            CcipTokenPoolError::RLDisabledNonZeroRateLimit
        );
    }
    Ok(())
}

#[event]
pub struct TokensConsumed {
    pub tokens: u64,
}

#[event]
pub struct ConfigChanged {
    pub config: RateLimitConfig,
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use super::*;

    #[derive(Default)]
    struct MockTimestamper;

    // Persistent state per test thread.
    thread_local! {
        static TIMESTAMP: RefCell<u64> = Default::default();
    }

    impl Timestamper for MockTimestamper {
        fn unix_timestamp(&self) -> u64 {
            TIMESTAMP.with(|ts| *ts.borrow())
        }

        fn instance() -> std::result::Result<Self, ProgramError> {
            Ok(Self)
        }
    }

    fn elapse(seconds: u64) {
        TIMESTAMP.with(|ts| *ts.borrow_mut() += seconds)
    }

    #[test]
    fn bucket_grows_even_during_failing_attempts() {
        let mut bucket = RateLimitTokenBucket {
            tokens: 0,
            last_updated: 0,
            cfg: RateLimitConfig {
                enabled: true,
                capacity: 50,
                // Refills one token per second
                rate: 1,
            },
        };

        for _ in 0..5 {
            assert_eq!(
                bucket.consume::<MockTimestamper>(5),
                Err(CcipTokenPoolError::RLRateLimitReached.into())
            );
            elapse(1);
        }
        // Finally enough time has passed and the bucket is full.
        bucket.consume::<MockTimestamper>(5).unwrap();
    }

    fn set_timestamp(ts: u64) {
        TIMESTAMP.with(|t| *t.borrow_mut() = ts)
    }

    fn make_bucket(
        tokens: u64,
        last_updated: u64,
        capacity: u64,
        rate: u64,
    ) -> RateLimitTokenBucket {
        RateLimitTokenBucket {
            tokens,
            last_updated,
            cfg: RateLimitConfig {
                enabled: true,
                capacity,
                rate,
            },
        }
    }

    #[test]
    fn as_of_now_refills_without_mutating_stored_bucket() {
        let bucket = make_bucket(10, 0, 50, 1);
        set_timestamp(5);

        let observed = bucket.as_of_now::<MockTimestamper>().unwrap();

        assert_eq!(observed.tokens, 15);
        assert_eq!(observed.last_updated, 5);
        // The stored bucket is untouched: observing must not consume nor persist a refill.
        assert_eq!(bucket.tokens, 10);
        assert_eq!(bucket.last_updated, 0);
    }

    #[test]
    fn as_of_now_caps_at_capacity() {
        let bucket = make_bucket(0, 0, 50, 10);
        set_timestamp(1_000);

        assert_eq!(bucket.as_of_now::<MockTimestamper>().unwrap().tokens, 50);
    }

    #[test]
    fn extreme_config_saturates_at_capacity() {
        // rate == capacity == u64::MAX, elapse 2s → saturating_mul caps at u64::MAX,
        // then min(capacity, refill) brings it back to capacity
        let mut bucket = make_bucket(0, 0, u64::MAX, u64::MAX);
        set_timestamp(2);
        bucket.refill::<MockTimestamper>().unwrap();
        assert_eq!(bucket.tokens, u64::MAX);
    }

    #[test]
    fn last_updated_greater_than_current_timestamp() {
        // last_updated > current_timestamp → saturating_sub gives 0, no refill
        let mut bucket = make_bucket(10, 100, 50, 1);
        set_timestamp(50);
        bucket.refill::<MockTimestamper>().unwrap();
        assert_eq!(bucket.tokens, 10); // unchanged
    }

    #[test]
    fn wait_time_with_extreme_values_returns_rate_limit() {
        // capacity = u64::MAX, rate = u64::MAX, request near max but bucket nearly empty
        // The addition overflow in wait-time defaults to u64::MAX, then returns RLRateLimitReached
        let mut bucket = make_bucket(0, 0, u64::MAX, u64::MAX);
        set_timestamp(0);
        // Bucket has 0 tokens, requesting u64::MAX → wait-time path
        let result = bucket.consume::<MockTimestamper>(u64::MAX);
        assert_eq!(result, Err(CcipTokenPoolError::RLRateLimitReached.into()));
    }

    #[test]
    fn request_max_tokens_with_low_balance() {
        // capacity == u64::MAX, 1 token available, request u64::MAX
        let mut bucket = make_bucket(1, 0, u64::MAX, 1);
        set_timestamp(0);
        let result = bucket.consume::<MockTimestamper>(u64::MAX);
        assert_eq!(result, Err(CcipTokenPoolError::RLRateLimitReached.into()));
    }

    #[test]
    fn refill_saturates_at_capacity() {
        // Large rate, enough time → tokens saturate at capacity, not overflow
        let mut bucket = make_bucket(0, 0, 1000, u64::MAX);
        set_timestamp(1);
        bucket.refill::<MockTimestamper>().unwrap();
        assert_eq!(bucket.tokens, 1000); // capped at capacity
    }

    #[test]
    fn consume_after_full_refill_works() {
        let mut bucket = make_bucket(0, 0, 100, 10);
        set_timestamp(10);
        bucket.consume::<MockTimestamper>(100).unwrap();
        assert_eq!(bucket.tokens, 0);
    }

    #[test]
    fn consume_with_overflow_refill_hits_rate_limit() {
        // Extreme rate causes refill overflow, but saturating math caps at capacity.
        // Request exceeds available tokens → returns RLRateLimitReached (not arithmetic error).
        let mut bucket = make_bucket(0, 0, 100, u64::MAX);
        set_timestamp(1); // refill saturates to u64::MAX, capped to capacity=100
        let result = bucket.consume::<MockTimestamper>(101);
        assert_eq!(
            result,
            Err(CcipTokenPoolError::RLMaxCapacityExceeded.into())
        );
    }

    #[test]
    fn consume_with_time_underflow_refill_no_panic() {
        // last_updated > current_timestamp → saturating refill adds 0.
        // Bucket has fewer tokens than requested → returns RLRateLimitReached.
        let mut bucket = make_bucket(5, 100, 50, 1);
        set_timestamp(50); // time_diff saturates to 0, no refill
        let result = bucket.consume::<MockTimestamper>(10);
        assert_eq!(result, Err(CcipTokenPoolError::RLRateLimitReached.into()));
        assert_eq!(bucket.tokens, 5); // unchanged
    }

    #[test]
    fn consume_disabled_is_noop() {
        let mut bucket = RateLimitTokenBucket {
            tokens: 0,
            last_updated: 0,
            cfg: RateLimitConfig {
                enabled: false,
                capacity: 0,
                rate: 0,
            },
        };
        set_timestamp(0);
        bucket.consume::<MockTimestamper>(1000).unwrap();
        assert_eq!(bucket.tokens, 0);
    }

    #[test]
    fn consume_zero_tokens_is_noop() {
        let mut bucket = make_bucket(10, 0, 100, 1);
        set_timestamp(0);
        bucket.consume::<MockTimestamper>(0).unwrap();
        assert_eq!(bucket.tokens, 10);
    }

    #[test]
    fn consume_exceeding_capacity_fails() {
        let mut bucket = make_bucket(100, 0, 100, 1);
        set_timestamp(0);
        let result = bucket.consume::<MockTimestamper>(101);
        assert_eq!(
            result,
            Err(CcipTokenPoolError::RLMaxCapacityExceeded.into())
        );
    }

    #[test]
    fn consume_with_zero_rate_and_insufficient_tokens() {
        // rate = 0, tokens < request → RLRateLimitReached (rate == 0 branch)
        let mut bucket = make_bucket(5, 0, 100, 0);
        set_timestamp(0);
        let result = bucket.consume::<MockTimestamper>(10);
        assert_eq!(result, Err(CcipTokenPoolError::RLRateLimitReached.into()));
    }

    #[test]
    fn validate_config_rate_exceeds_capacity() {
        let mut bucket = make_bucket(0, 0, 10, 1);
        let result = bucket.set_token_bucket_config(RateLimitConfig {
            enabled: true,
            capacity: 10,
            rate: 11,
        });
        assert_eq!(
            result,
            Err(CcipTokenPoolError::RLInvalidRateLimitRate.into())
        );
    }

    #[test]
    fn validate_config_disabled_nonzero_values() {
        let mut bucket = make_bucket(0, 0, 10, 1);
        let result = bucket.set_token_bucket_config(RateLimitConfig {
            enabled: false,
            capacity: 10,
            rate: 0,
        });
        assert_eq!(
            result,
            Err(CcipTokenPoolError::RLDisabledNonZeroRateLimit.into())
        );
    }

    fn enabled(capacity: u64, rate: u64) -> RateLimitConfig {
        RateLimitConfig {
            enabled: true,
            capacity,
            rate,
        }
    }

    #[test]
    fn reconfig_resets_a_drained_bucket_to_the_new_capacity() {
        let mut bucket = make_bucket(100, 0, 100, 1);
        set_timestamp(10);
        bucket.consume::<MockTimestamper>(100).unwrap();
        assert_eq!(bucket.tokens, 0);

        // Same second: no time has passed, yet the bucket is full under the new config.
        bucket
            .set_token_bucket_config_at::<MockTimestamper>(enabled(500, 5))
            .unwrap();
        assert_eq!(bucket.tokens, 500);
        assert_eq!(bucket.last_updated, 10);
    }

    #[test]
    fn reconfig_ignores_time_elapsed_before_the_update() {
        let mut bucket = make_bucket(0, 0, 1_000, 10);
        set_timestamp(100_000);

        bucket
            .set_token_bucket_config_at::<MockTimestamper>(enabled(5_000, 1_000))
            .unwrap();
        assert_eq!(bucket.tokens, 5_000);
        assert_eq!(bucket.last_updated, 100_000);

        // Refill restarts from the update, at the new rate.
        bucket.consume::<MockTimestamper>(5_000).unwrap();
        elapse(2);
        bucket.refill::<MockTimestamper>().unwrap();
        assert_eq!(bucket.tokens, 2_000);
    }

    #[test]
    fn reconfig_to_a_lower_capacity_resets_to_it() {
        let mut bucket = make_bucket(1_000, 0, 1_000, 10);
        set_timestamp(0);

        bucket
            .set_token_bucket_config_at::<MockTimestamper>(enabled(500, 5))
            .unwrap();
        assert_eq!(bucket.tokens, 500);
    }

    #[test]
    fn enabling_a_disabled_limit_starts_full() {
        let mut bucket = RateLimitTokenBucket {
            tokens: 0,
            last_updated: 0,
            cfg: RateLimitConfig {
                enabled: false,
                capacity: 0,
                rate: 0,
            },
        };
        set_timestamp(0);

        bucket
            .set_token_bucket_config_at::<MockTimestamper>(enabled(1_000, 10))
            .unwrap();
        bucket.consume::<MockTimestamper>(1_000).unwrap();
    }

    #[test]
    fn disabling_a_limit_empties_the_bucket() {
        let mut bucket = make_bucket(1_000, 0, 1_000, 10);
        set_timestamp(0);

        bucket
            .set_token_bucket_config_at::<MockTimestamper>(RateLimitConfig {
                enabled: false,
                capacity: 0,
                rate: 0,
            })
            .unwrap();
        assert_eq!(bucket.tokens, 0);
        assert!(!bucket.is_enabled());
    }

    #[test]
    fn tokens_never_exceed_capacity() {
        // Stress: repeated refills should never push tokens above capacity
        let mut bucket = make_bucket(99, 0, 100, 50);
        for t in 1..=10 {
            set_timestamp(t);
            bucket.refill::<MockTimestamper>().unwrap();
            assert!(bucket.tokens <= bucket.cfg.capacity);
        }
    }
}
