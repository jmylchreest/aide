import { BrowserRouter, Routes, Route } from "react-router-dom";
import { ClipboardProvider } from "@/context/ClipboardContext";
import { AppLayout } from "./layout/AppLayout";
import { InstancesPage } from "./pages/InstancesPage";
import { StatusPage } from "./pages/StatusPage";
import { MemoriesPage } from "./pages/MemoriesPage";
import { DecisionsPage } from "./pages/DecisionsPage";
import { TasksPage } from "./pages/TasksPage";
import { MessagesPage } from "./pages/MessagesPage";
import { StatePage } from "./pages/StatePage";
import { CodePage } from "./pages/CodePage";
import { FindingsPage } from "./pages/FindingsPage";
import { SurveyPage } from "./pages/SurveyPage";
import { TokensPage } from "./pages/TokensPage";
import { SearchPage } from "./pages/SearchPage";

export default function App() {
  return (
    <ClipboardProvider>
    <BrowserRouter>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<InstancesPage />} />
          <Route path="search" element={<SearchPage />} />
          <Route path="instances/:project">
            <Route path="status" element={<StatusPage />} />
            <Route path="memories" element={<MemoriesPage />} />
            <Route path="decisions" element={<DecisionsPage />} />
            <Route path="tasks" element={<TasksPage />} />
            <Route path="messages" element={<MessagesPage />} />
            <Route path="state" element={<StatePage />} />
            <Route path="code" element={<CodePage />} />
            <Route path="findings" element={<FindingsPage />} />
            <Route path="survey" element={<SurveyPage />} />
            <Route path="tokens" element={<TokensPage />} />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
    </ClipboardProvider>
  );
}
