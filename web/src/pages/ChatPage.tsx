import { useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { ActionFeedback, Freshness, PageHeader, RelativeTime, StatePanel, StatusPill } from "../components/Common";
import { useCollection } from "../hooks/useCollection";

export function ChatPage() {
  const { projectId } = useAuth();
  const collection = useCollection(api.messages);
  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "nearest" });
  }, [collection.items.length]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const text = body.trim();
    if (!projectId || !text) return;
    setSending(true);
    setMessage(null);
    setError(null);
    try {
      await api.sendMessage(projectId, text);
      setBody("");
      setMessage("Message posted.");
      await collection.reload();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The message could not be posted.");
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="page page--chat">
      <PageHeader
        eyebrow="Shared supervisory surface"
        title="Chat"
        description="Talk with human teammates and authorized participants such as Pip. Conversation never silently becomes a command."
      />
      <div className="inline-notice">
        <strong>Chat is not a command bus.</strong> Action-like language stays a message until an authorized structured action is reviewed and submitted.
      </div>
      <Freshness loadedAt={collection.lastLoadedAt} />
      <ActionFeedback message={message} error={error} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No conversation yet"
        emptyBody="Start a project conversation. Pip will appear with an explicit agent identity when Hermes is connected."
        onRetry={() => void collection.reload()}
      >
        <section className="chat-log" aria-label="Project messages" aria-live="polite">
          {collection.items.map((chat) => {
            const pip = chat.author_id === "agent_pip";
            const author = pip ? "Pip" : chat.author_id;
            return (
              <article className={`chat-message ${pip ? "chat-message--pip" : ""}`} key={chat.id}>
                <div className="avatar">{author.slice(0, 2).toUpperCase()}</div>
                <div>
                  <header>
                    <strong>{author}</strong>
                    <StatusPill value={pip ? "Pip · Hermes agent" : chat.role} />
                    <RelativeTime value={chat.created_at} />
                  </header>
                  <p>{chat.body}</p>
                  {chat.session_id || chat.run_id ? <small>{chat.session_id ? `session ${chat.session_id}` : ""}{chat.run_id ? ` · run ${chat.run_id}` : ""}</small> : null}
                </div>
              </article>
            );
          })}
          <div ref={endRef} />
        </section>
      </StatePanel>
      <form className="chat-composer" onSubmit={(event) => void submit(event)}>
        <label htmlFor="chat-body">Message this project</label>
        <div>
          <textarea
            id="chat-body"
            value={body}
            onChange={(event) => setBody(event.target.value)}
            required
            maxLength={4000}
            placeholder="Share context or ask Pip a question…"
          />
          <button className="button button--primary" type="submit" disabled={sending || !body.trim()}>
            {sending ? "Sending…" : "Send message"}
          </button>
        </div>
        <small>{body.length}/4,000 · Messages cannot execute controls.</small>
      </form>
    </div>
  );
}
