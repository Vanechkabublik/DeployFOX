export function Badge({ status }: { status: string }) {
  const good = ["RUNNING", "SUCCESS"].includes(status);
  const bad = status === "FAILED";
  return (
    <span className={`badge ${good ? "good" : bad ? "bad" : "neutral"}`}>
      <i />
      {status.toLowerCase()}
    </span>
  );
}
