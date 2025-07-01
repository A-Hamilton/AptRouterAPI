-- AptRouter API Initial Database Schema
-- This migration creates all the necessary tables, functions, and security policies

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create profiles table
CREATE TABLE IF NOT EXISTS public.profiles (
    id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    email TEXT NOT NULL UNIQUE,
    balance NUMERIC(10,2) NOT NULL DEFAULT 100.00,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create api_keys table for AptRouter's own service keys
CREATE TABLE IF NOT EXISTS public.api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES public.profiles(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'revoked')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create pricing_tiers table
CREATE TABLE IF NOT EXISTS public.pricing_tiers (
    id SERIAL PRIMARY KEY,
    tier_name TEXT NOT NULL UNIQUE,
    min_monthly_requests INTEGER NOT NULL DEFAULT 0,
    input_savings_rate_usd_per_million NUMERIC(10,6) NOT NULL,
    output_savings_rate_usd_per_million NUMERIC(10,6) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create model_configs table
CREATE TABLE IF NOT EXISTS public.model_configs (
    id SERIAL PRIMARY KEY,
    model_id TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    input_price_per_million NUMERIC(10,6) NOT NULL,
    output_price_per_million NUMERIC(10,6) NOT NULL,
    context_window_size INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create request_logs table (comprehensive audit log)
CREATE TABLE IF NOT EXISTS public.request_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES public.profiles(id) ON DELETE CASCADE,
    api_key_id UUID NOT NULL REFERENCES public.api_keys(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    was_optimized BOOLEAN NOT NULL DEFAULT false,
    original_input_tokens INTEGER,
    optimized_input_tokens INTEGER,
    actual_input_tokens INTEGER NOT NULL,
    actual_output_tokens INTEGER,
    output_tokens_saved_est INTEGER DEFAULT 0,
    usage_is_estimated BOOLEAN NOT NULL DEFAULT false,
    fallback_reason TEXT,
    optimization_status TEXT,
    cost_usd NUMERIC(10,6) NOT NULL,
    fee_charged_usd NUMERIC(10,6) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create transactions table
CREATE TABLE IF NOT EXISTS public.transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES public.profiles(id) ON DELETE CASCADE,
    request_log_id UUID NOT NULL REFERENCES public.request_logs(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('charge', 'refund', 'credit')),
    amount NUMERIC(10,6) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON public.api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON public.api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix ON public.api_keys(key_prefix);
CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON public.request_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON public.request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON public.transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_request_log_id ON public.transactions(request_log_id);

-- Create the atomic charge_user_for_request function
CREATE OR REPLACE FUNCTION public.charge_user_for_request(
    p_user_id UUID,
    p_api_key_id UUID,
    p_request_id TEXT,
    p_model_id TEXT,
    p_provider TEXT,
    p_was_optimized BOOLEAN,
    p_original_input_tokens INTEGER,
    p_optimized_input_tokens INTEGER,
    p_actual_input_tokens INTEGER,
    p_actual_output_tokens INTEGER,
    p_output_tokens_saved_est INTEGER,
    p_usage_is_estimated BOOLEAN,
    p_fallback_reason TEXT,
    p_optimization_status TEXT,
    p_cost_usd NUMERIC,
    p_fee_charged_usd NUMERIC
) RETURNS UUID AS $$
DECLARE
    v_request_log_id UUID;
    v_current_balance NUMERIC;
    v_new_balance NUMERIC;
BEGIN
    -- Start transaction
    BEGIN
        -- Check user balance
        SELECT balance INTO v_current_balance 
        FROM public.profiles 
        WHERE id = p_user_id 
        FOR UPDATE;
        
        IF NOT FOUND THEN
            RAISE EXCEPTION 'User not found';
        END IF;
        
        -- Calculate new balance
        v_new_balance := v_current_balance - p_cost_usd;
        
        -- Check if user has sufficient balance
        IF v_new_balance < 0 THEN
            RAISE EXCEPTION 'Insufficient balance';
        END IF;
        
        -- Insert request log
        INSERT INTO public.request_logs (
            user_id, api_key_id, request_id, model_id, provider,
            was_optimized, original_input_tokens, optimized_input_tokens,
            actual_input_tokens, actual_output_tokens, output_tokens_saved_est,
            usage_is_estimated, fallback_reason, optimization_status,
            cost_usd, fee_charged_usd
        ) VALUES (
            p_user_id, p_api_key_id, p_request_id, p_model_id, p_provider,
            p_was_optimized, p_original_input_tokens, p_optimized_input_tokens,
            p_actual_input_tokens, p_actual_output_tokens, p_output_tokens_saved_est,
            p_usage_is_estimated, p_fallback_reason, p_optimization_status,
            p_cost_usd, p_fee_charged_usd
        ) RETURNING id INTO v_request_log_id;
        
        -- Update user balance
        UPDATE public.profiles 
        SET balance = v_new_balance, updated_at = NOW()
        WHERE id = p_user_id;
        
        -- Insert transaction record
        INSERT INTO public.transactions (
            user_id, request_log_id, type, amount, description
        ) VALUES (
            p_user_id, v_request_log_id, 'charge', p_cost_usd,
            'API usage charge for ' || p_model_id
        );
        
        -- If there's a fee charged (savings), create a separate transaction
        IF p_fee_charged_usd > 0 THEN
            INSERT INTO public.transactions (
                user_id, request_log_id, type, amount, description
            ) VALUES (
                p_user_id, v_request_log_id, 'charge', p_fee_charged_usd,
                'Optimization fee for token savings'
            );
        END IF;
        
        RETURN v_request_log_id;
        
    EXCEPTION
        WHEN OTHERS THEN
            -- Rollback transaction on any error
            RAISE;
    END;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Enable Row Level Security on all tables
ALTER TABLE public.profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.request_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.transactions ENABLE ROW LEVEL SECURITY;

-- RLS Policies for profiles
CREATE POLICY "Users can view own profile" ON public.profiles
    FOR SELECT USING (auth.uid() = id);

CREATE POLICY "Users can update own profile" ON public.profiles
    FOR UPDATE USING (auth.uid() = id);

-- RLS Policies for api_keys
CREATE POLICY "Users can view own api keys" ON public.api_keys
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own api keys" ON public.api_keys
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own api keys" ON public.api_keys
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own api keys" ON public.api_keys
    FOR DELETE USING (auth.uid() = user_id);

-- RLS Policies for request_logs
CREATE POLICY "Users can view own request logs" ON public.request_logs
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Service can insert request logs" ON public.request_logs
    FOR INSERT WITH CHECK (true); -- Service role will bypass RLS

-- RLS Policies for transactions
CREATE POLICY "Users can view own transactions" ON public.transactions
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Service can insert transactions" ON public.transactions
    FOR INSERT WITH CHECK (true); -- Service role will bypass RLS

-- Insert default pricing tiers
INSERT INTO public.pricing_tiers (tier_name, min_monthly_requests, input_savings_rate_usd_per_million, output_savings_rate_usd_per_million) VALUES
    ('free', 0, 0.50, 1.50),
    ('starter', 1000, 0.40, 1.20),
    ('pro', 10000, 0.30, 0.90),
    ('enterprise', 100000, 0.20, 0.60)
ON CONFLICT (tier_name) DO NOTHING;

-- Insert default model configurations
INSERT INTO public.model_configs (model_id, provider, input_price_per_million, output_price_per_million, context_window_size) VALUES
    ('gpt-4o', 'openai', 5.00, 15.00, 128000),
    ('gpt-4o-mini', 'openai', 0.15, 0.60, 128000),
    ('gpt-3.5-turbo', 'openai', 0.50, 1.50, 16385),
    ('claude-3-5-sonnet-20241022', 'anthropic', 3.00, 15.00, 200000),
    ('claude-3-5-haiku-20241022', 'anthropic', 0.25, 1.25, 200000),
    ('gemini-1.5-pro', 'google', 3.50, 10.50, 1000000),
    ('gemini-1.5-flash', 'google', 0.075, 0.30, 1000000)
ON CONFLICT (model_id) DO NOTHING;

-- Create function to get user's pricing tier based on monthly request count
CREATE OR REPLACE FUNCTION public.get_user_pricing_tier(p_user_id UUID)
RETURNS TEXT AS $$
DECLARE
    v_monthly_requests INTEGER;
    v_tier_name TEXT;
BEGIN
    -- Count requests in the last 30 days
    SELECT COUNT(*) INTO v_monthly_requests
    FROM public.request_logs
    WHERE user_id = p_user_id
    AND created_at >= NOW() - INTERVAL '30 days';
    
    -- Find the appropriate tier
    SELECT tier_name INTO v_tier_name
    FROM public.pricing_tiers
    WHERE min_monthly_requests <= v_monthly_requests
    ORDER BY min_monthly_requests DESC
    LIMIT 1;
    
    RETURN COALESCE(v_tier_name, 'free');
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Create function to get user's monthly request count
CREATE OR REPLACE FUNCTION public.get_user_monthly_requests(p_user_id UUID)
RETURNS INTEGER AS $$
DECLARE
    v_monthly_requests INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_monthly_requests
    FROM public.request_logs
    WHERE user_id = p_user_id
    AND created_at >= NOW() - INTERVAL '30 days';
    
    RETURN COALESCE(v_monthly_requests, 0);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER; 