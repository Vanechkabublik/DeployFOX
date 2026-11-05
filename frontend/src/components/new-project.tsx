"use client";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useState } from "react";
import { Box, Github, X, ArrowRight } from "lucide-react";
import { api, post } from "@/lib/api";
import type { Project } from "@/types";
import { RepositoryPicker } from "./repository-picker";
const schema = z.object({
  name: z.string().min(2).max(64),
  source_type: z.enum(["image", "git"]),
  service_type: z.enum(["http", "worker"]),
  source: z.string().min(2),
  branch: z.string().min(1),
  domain: z.string(),
  port: z.number().min(1).max(65535),
  memory_mb: z.number().min(64).max(32768),
  cpu: z.number().positive().max(32),
  health_path: z.string().startsWith("/"),
  dockerfile: z.string().min(1),
  build_context: z.string().min(1),
  auto_deploy: z.boolean(),
});
type Input = z.infer<typeof schema>;
export function NewProject({
  close,
  saved,
  initial,
}: {
  close: () => void;
  saved: () => void;
  initial?: Project;
}) {
  const [error, setError] = useState("");
  const {
    register,
    handleSubmit,
    control,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<Input>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: "",
      source_type: "image",
      service_type: "http",
      source: "",
      branch: "main",
      domain: "",
      port: 3000,
      memory_mb: 512,
      cpu: 1,
      health_path: "/",
      dockerfile: "Dockerfile",
      build_context: ".",
      auto_deploy: false,
      ...initial,
    },
  });
  const type = useWatch({ control, name: "source_type" });
  const source = useWatch({ control, name: "source" });
  return (
    <div className="overlay" onClick={close}>
      <section
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-heading">
          <div>
            <span className="eyebrow">LET’S SHIP SOMETHING</span>
            <h2 id="new-title">
              {initial ? "Настройки проекта" : "Новый проект"}
            </h2>
          </div>
          <button className="icon-button" aria-label="Закрыть" onClick={close}>
            <X />
          </button>
        </div>
        <form
          onSubmit={handleSubmit(async (data) => {
            setError("");
            try {
              if (initial) {
                await api(`/projects/${initial.id}`, {
                  method: "PUT",
                  body: JSON.stringify({ ...data, build_method: "dockerfile" }),
                });
              } else {
                await post("/projects", {
                  ...data,
                  build_method: "dockerfile",
                });
              }
              saved();
            } catch (err) {
              setError((err as Error).message);
            }
          })}
        >
          <label>
            Тип приложения
            <select
              {...register("service_type")}
              onChange={(event) => {
                setValue(
                  "service_type",
                  event.target.value === "worker" ? "worker" : "http",
                );
                if (event.target.value === "worker") setValue("domain", "");
              }}
            >
              <option value="http">HTTP-приложение · сайт или API</option>
              <option value="worker">Фоновый процесс · бот или worker</option>
            </select>
          </label>
          <div className="source-tabs">
            <button
              type="button"
              className={type === "image" ? "selected" : ""}
              onClick={() => setValue("source_type", "image")}
            >
              <Box size={19} /> Docker image
            </button>
            <button
              type="button"
              className={type === "git" ? "selected" : ""}
              onClick={() => setValue("source_type", "git")}
            >
              <Github size={19} /> GitHub repository
            </button>
          </div>
          {type === "git" && (
            <RepositoryPicker
              source={source}
              choose={(url, branch) => {
                setValue("source", url);
                setValue("branch", branch);
              }}
              branch={(value) => setValue("branch", value)}
            />
          )}
          <label>
            Название
            <input {...register("name")} placeholder="my-awesome-app" />
          </label>
          <label>
            {type === "image" ? "Docker image" : "GitHub HTTPS URL"}
            <input
              {...register("source")}
              placeholder={
                type === "image"
                  ? "nginx:alpine"
                  : "https://github.com/owner/repository"
              }
            />
          </label>
          {type === "git" && (
            <>
              <div className="form-grid">
                <label>
                  Ветка
                  <input {...register("branch")} />
                </label>
                <label>
                  Dockerfile
                  <input {...register("dockerfile")} />
                </label>
              </div>
              <label>
                Build context
                <input {...register("build_context")} />
              </label>
              <label className="check-label">
                <input type="checkbox" {...register("auto_deploy")} />{" "}
                Автодеплой при push в эту ветку
              </label>
              <p className="small muted">
                Приватные репозитории доступны через GitHub App, настроенный на
                сервере. Для сборки нужен Dockerfile.
              </p>
            </>
          )}
          <div className="form-grid">
            <label>
              Порт контейнера
              <input
                type="number"
                {...register("port", { valueAsNumber: true })}
              />
            </label>
            <label>
              Healthcheck path
              <input {...register("health_path")} />
            </label>
          </div>
          <label>
            Домен <span className="muted">· необязательно</span>
            <input {...register("domain")} placeholder="app.example.com" />
          </label>
          <div className="form-grid">
            <label>
              Память · MB
              <input
                type="number"
                {...register("memory_mb", { valueAsNumber: true })}
              />
            </label>
            <label>
              CPU
              <input
                type="number"
                step="0.25"
                {...register("cpu", { valueAsNumber: true })}
              />
            </label>
          </div>
          {Object.keys(errors).length > 0 && (
            <div className="error" role="alert">
              Проверьте поля: {Object.keys(errors).join(", ")}
            </div>
          )}
          {error && (
            <div className="error" role="alert">
              {error}
            </div>
          )}
          <div className="modal-footer">
            <span className="small muted">
              Только доверенные проекты владельца
            </span>
            <button className="primary" disabled={isSubmitting}>
              {isSubmitting
                ? "Сохраняем…"
                : initial
                  ? "Сохранить настройки"
                  : "Создать проект"}
              <ArrowRight size={16} />
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
