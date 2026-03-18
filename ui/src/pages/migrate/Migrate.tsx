import { useState } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { MigrateWizard } from './components/MigrateWizard'
import { JobCard } from './components/JobCard'
import { useMigrateJobs } from '../../hooks/useMigrate'

export function Migrate() {
  const [activeTab, setActiveTab] = useState('new')
  const { data: jobs, isLoading } = useMigrateJobs()

  return (
    <div className="p-6 h-full overflow-auto">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-foreground">Migrate</h1>
        <p className="text-muted-foreground">
          Import data from other analytics tools or convert tag manager containers
        </p>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="new">New Import</TabsTrigger>
          <TabsTrigger value="history">
            Import History
            {jobs && jobs.length > 0 && (
              <span className="ml-2 text-xs bg-muted px-1.5 py-0.5 rounded-full">
                {jobs.length}
              </span>
            )}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="new" className="mt-4">
          <MigrateWizard onComplete={() => setActiveTab('history')} />
        </TabsContent>

        <TabsContent value="history" className="mt-4">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
            </div>
          ) : jobs && jobs.length > 0 ? (
            <div className="space-y-3">
              {jobs.map((job) => (
                <JobCard key={job.id} job={job} />
              ))}
            </div>
          ) : (
            <div className="text-center py-12 text-muted-foreground">
              No imports yet. Start a new import to see it here.
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
