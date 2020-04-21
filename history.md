## History

### v0.10.0

- Initial workers automatically start running on pool initialization
    - deprecated StartWorkers()
    
- Each new added worker is being automatically started

- **SafeWaitUntilNSuccesses**: it waits until *n* tasks were successfully processed, but if any extra task is already "in progress", this function will wait until it is done. An extra enqueued task could started processing just before the *nth* expected task was finished.

- **GetTotalWorkersInProgress**: returns total workers in progress.

- **KillAllWorkers** returns error
- **KillAllWorkersAndWait** returns error

- **SetTotalWorkers** It won't return error because of the workers were not yet started, workers are now started once they are created.

- **WaitUntilInitialWorkersAreUp**: it waits until all initial workers are up and running.

- **StartWorkers** is deprecated. It only returns nil.
- **StartWorkersAndWait** is deprecated. It returns WaitUntilInitialWorkersAreUp()

### v0.9.1

- Examples: Replaced pool.StartWorkers() by pool.StartWorkersAndWait()

### v0.9.0

- Added optional channel to let know that new workers were started

### v0.8.0

 - Enqueue jobs plus callback functions
 - Enqueue callback functions without jobs' data

### v0.7.4

 - Fixed bug that caused randomly worker initialization error

### v0.7.3

 - SetTotalWorkers() returns error in case it is invoked before StartWorkers()

### v0.7.2

 - Fixed bug that prevents to start/add new workers after a Wait() function finishes.

### v0.7.1

 - LateKillAllWorkers() will kill all alive workers (not only the number of workers that were alive when the function was invoked)

### v0.7

 - Repository name modified to "goworkerpool"

### v0.6

 - Pause / Resume all workers:
   - PauseAllWorkers() 
   - ResumeAllWorkers()
 - Workers will listen to higher priority channels first
 - Workers will listen to broad messages (kill all workers, ...) before get signals from any other channel:
   - KillAllWorkers()
   - KillAllWorkersAndWait()
 - Added function to kill all workers (send a broad message to all workers) and wait until it happens:
   - pool.KillAllWorkersAndWait()
 - Added code examples
   
   
### v0.5

 - Make Wait() listen to a channel (instead of use an endless for loop)
 
### v0.4

 - Sync actions over workers. A FIFO queue was created for the following actions:
   - Add new worker
   - Kill worker(s)
   - Late kill worker(s)
   - Set total workers 
   
### v0.3

 - Added function to adjust number of live workers:
   - pool.SetTotalWorkers(n)
 - Added function to kill all live workers after current jobs get processed:
   - pool.LateKillAllWorkers()
   
### v0.2

 - readme.md
 - godoc
 - code comments

### v0.1

 First stable BETA version.
