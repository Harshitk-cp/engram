import { useState } from "react";
import { Api, BillingState, PlanOption } from "../api";
import { Layout } from "../components/Layout";
import { useAsync } from "../hooks";
import { AsyncView } from "../components/ui";

// Razorpay Checkout is loaded on demand from their CDN (it injects window.Razorpay).
interface RazorpaySuccess {
  razorpay_payment_id: string;
  razorpay_subscription_id: string;
  razorpay_signature: string;
}
interface RazorpayInstance {
  open: () => void;
  on: (event: string, cb: (resp: unknown) => void) => void;
}
declare global {
  interface Window {
    Razorpay: new (options: Record<string, unknown>) => RazorpayInstance;
  }
}

const RZP_SRC = "https://checkout.razorpay.com/v1/checkout.js";
let rzpScriptPromise: Promise<void> | null = null;

// loadRazorpay injects the Checkout script once (promise-cached) and resolves when
// window.Razorpay is available.
function loadRazorpay(): Promise<void> {
  if (typeof window !== "undefined" && window.Razorpay) return Promise.resolve();
  if (rzpScriptPromise) return rzpScriptPromise;
  rzpScriptPromise = new Promise<void>((resolve, reject) => {
    const s = document.createElement("script");
    s.src = RZP_SRC;
    s.onload = () => resolve();
    s.onerror = () => {
      rzpScriptPromise = null;
      reject(new Error("Failed to load Razorpay Checkout"));
    };
    document.body.appendChild(s);
  });
  return rzpScriptPromise;
}

const PLAN_LABELS: Record<string, string> = {
  free: "Free",
  developer: "Developer",
  team: "Team",
  growth: "Growth",
  enterprise: "Enterprise",
};

function fmt(n: number): string {
  if (n < 0) return "Unlimited";
  return n.toLocaleString();
}

function UsageBar({ label, used, limit }: { label: string; used: number; limit: number }) {
  const unlimited = limit < 0;
  const pct = unlimited || limit === 0 ? 0 : Math.min(100, Math.round((used / limit) * 100));
  const warn = pct >= 80;
  return (
    <div className="tier-row" style={{ marginBottom: 10 }}>
      <span className="tier-label" style={{ minWidth: 130 }}>{label}</span>
      <div className="bar">
        <div className={`bar-fill ${warn ? "warm" : "hot"}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="tier-count">
        {fmt(used)} {unlimited ? "" : `/ ${fmt(limit)}`}
      </span>
    </div>
  );
}

function PlanCard({
  option,
  current,
  onUpgrade,
  busy,
}: {
  option: PlanOption;
  current: boolean;
  onUpgrade: (plan: string) => void;
  busy: boolean;
}) {
  const { plan, limits, purchasable } = option;
  return (
    <div className="card" style={{ flex: 1, minWidth: 200, borderColor: current ? "var(--accent)" : undefined }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h3 style={{ margin: 0 }}>{PLAN_LABELS[plan] ?? plan}</h3>
        {current && <span className="badge">current</span>}
      </div>
      <div style={{ fontSize: "1.6rem", fontWeight: 700, margin: "10px 0" }}>
        ${limits.price_usd}
        <span className="secondary" style={{ fontSize: "0.85rem", fontWeight: 400 }}>/mo</span>
      </div>
      <ul className="secondary" style={{ paddingLeft: 18, margin: "8px 0 16px", lineHeight: 1.7 }}>
        <li>{fmt(limits.max_agents)} agents</li>
        <li>{fmt(limits.max_memories_per_month)} memories / mo</li>
      </ul>
      <button
        className="btn"
        disabled={current || busy || !purchasable}
        onClick={() => onUpgrade(plan)}
        style={{ width: "100%" }}
      >
        {current ? "Current plan" : purchasable ? "Upgrade" : "Unavailable"}
      </button>
    </div>
  );
}

function BillingInner({ data, reload }: { data: BillingState; reload: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // onUpgrade creates a Razorpay subscription server-side, then opens the Checkout
  // modal. On a successful payment the signature is verified server-side and the
  // billing state reloads; the subscription.* webhook is the authoritative source.
  const onUpgrade = async (plan: string) => {
    setBusy(true);
    setErr(null);
    try {
      await loadRazorpay();
      const { subscription_id, key_id } = await Api.checkout(plan);
      const rzp = new window.Razorpay({
        key: key_id,
        subscription_id,
        name: "Engram",
        description: `${PLAN_LABELS[plan] ?? plan} plan`,
        theme: { color: "#6366f1" },
        handler: async (resp: RazorpaySuccess) => {
          try {
            await Api.verifyPayment({
              razorpay_payment_id: resp.razorpay_payment_id,
              razorpay_subscription_id: resp.razorpay_subscription_id,
              razorpay_signature: resp.razorpay_signature,
            });
            reload();
          } catch (e) {
            setErr(e instanceof Error ? e.message : String(e));
          } finally {
            setBusy(false);
          }
        },
        modal: { ondismiss: () => setBusy(false) },
      });
      rzp.on("payment.failed", (resp: unknown) => {
        const r = resp as { error?: { description?: string } };
        setErr(r?.error?.description ?? "Payment failed");
        setBusy(false);
      });
      rzp.open();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };

  const onCancel = async () => {
    if (
      !window.confirm(
        "Cancel your subscription? You'll keep access until the end of the current billing period, then drop to the Free plan.",
      )
    )
      return;
    setBusy(true);
    setErr(null);
    try {
      await Api.cancelSubscription();
      reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="page-head">
        <h2>Billing</h2>
        <p>Your plan, usage this month, and subscription.</p>
      </div>

      {!data.billing_enabled && (
        <div className="card" style={{ marginBottom: 18 }}>
          <strong>Self-hosted mode.</strong>{" "}
          <span className="secondary">
            Billing is not configured on this server, so usage is unmetered and plan limits are not enforced.
          </span>
        </div>
      )}

      <div className="card" style={{ marginBottom: 22 }}>
        <div className="row" style={{ justifyContent: "space-between", marginBottom: 16 }}>
          <div>
            <div className="secondary" style={{ fontSize: "0.8rem" }}>Current plan</div>
            <div style={{ fontSize: "1.25rem", fontWeight: 700 }}>
              {PLAN_LABELS[data.plan] ?? data.plan}
              {data.subscription_status && data.subscription_status !== "active" && (
                <span className="pill" style={{ marginLeft: 8 }}>{data.subscription_status}</span>
              )}
            </div>
          </div>
          {data.billing_enabled && data.plan !== "free" && data.subscription_status !== "canceled" && (
            <button className="secondary" disabled={busy} onClick={onCancel}>
              Cancel subscription
            </button>
          )}
        </div>
        <UsageBar label="Memories (this mo.)" used={data.usage.memories_written} limit={data.limits.max_memories_per_month} />
        <UsageBar label="Agents" used={data.agent_count} limit={data.limits.max_agents} />
      </div>

      {err && <div className="error-box" style={{ marginBottom: 16 }}>{err}</div>}

      {data.billing_enabled && (
        <>
          <h3 style={{ margin: "0 0 12px" }}>Plans</h3>
          <div className="row" style={{ gap: 16, alignItems: "stretch", flexWrap: "wrap" }}>
            {data.plans.map((p) => (
              <PlanCard
                key={p.plan}
                option={p}
                current={p.plan === data.plan}
                busy={busy}
                onUpgrade={onUpgrade}
              />
            ))}
          </div>
          <p className="secondary" style={{ marginTop: 16, fontSize: "0.85rem" }}>
            Need more, SSO, or a self-hosted deployment? <a href="mailto:sales@hakuya.ai">Contact us</a> about Enterprise.
          </p>
        </>
      )}
    </>
  );
}

export default function Billing() {
  const state = useAsync(() => Api.billing(), []);
  return (
    <Layout title="Billing">
      <AsyncView state={state}>{(data) => <BillingInner data={data} reload={state.reload} />}</AsyncView>
    </Layout>
  );
}
