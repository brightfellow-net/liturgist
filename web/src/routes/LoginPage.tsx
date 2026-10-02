// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { LoginForm } from "@/components/LoginForm";
import { setupStatusQuery } from "@/lib/queries";
import { PublicLayout } from "./PublicLayout";
import { paths, safeNext } from "./paths";

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const queryClient = useQueryClient();
  const [forgot, setForgot] = useState(false);
  const status = useQuery(setupStatusQuery);

  if (status.data && !status.data.set_up) return <Navigate to={paths.setup} replace />;
  return (
    <PublicLayout title={t("login.title")}>
      <LoginForm
        submitLabel={t("login.submit")}
        onSuccess={async () => {
          queryClient.clear();
          await navigate(safeNext(params.get("next")), { replace: true });
        }}
      />
      <div className="space-y-2">
        <Button variant="link" aria-expanded={forgot} onClick={() => setForgot(!forgot)}>{t("login.forgot")}</Button>
        {forgot && <Alert>{t("login.forgot_text")}</Alert>}
      </div>
    </PublicLayout>
  );
}
