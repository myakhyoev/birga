-- Sign-up no longer verifies the phone number with an OTP; every new user starts as unverified_user.
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'unverified_user';
